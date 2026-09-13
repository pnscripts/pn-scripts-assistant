/*
 * Package tunnel is how a device reaches this brain from outside the house.
 *
 * WireGuard, and deliberately WireGuard rather than anything with a company
 * behind it. A tunnel puts the phone on the home network; nothing new is
 * exposed to the internet, the brain goes on answering only its own network,
 * and everything already built — the pairing, the tokens, the decisions
 * staying at the desk — keeps working unchanged because from its point of view
 * nothing has changed at all.
 *
 * The alternative was a relay: a machine on the internet forwarding to the
 * house. That is a public front door in front of somebody's memory, their
 * mail, and the ability to run commands on their computer, and it is a machine
 * they would have to keep patched forever.
 *
 * No new dependency. The keys are X25519, which the standard library has had
 * since Go 1.20, and the tunnel itself is the kernel's — this writes the
 * configuration and lets the operating system do what it is for. A userspace
 * implementation would have meant pulling in a network stack the size of the
 * rest of the program, into the one program whose whole argument is that you
 * can see what is in it.
 */
package tunnel

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileName is where the tunnel lives, in the brain's own folder, so it travels
// with the brain and goes when it goes.
const FileName = "tunnel.json"

// Interface is what the tunnel is called on this machine. Its own name rather
// than wg0, so it cannot collide with a tunnel somebody already had.
const Interface = "pnassistant"

/*
 * Network is the addresses inside the tunnel.
 *
 * Chosen from the middle of the private range rather than 10.0.0.x or
 * 192.168.1.x, because those are what home routers use — and a tunnel whose
 * addresses collide with the café's wifi is a tunnel that works everywhere
 * except where somebody actually needs it.
 */
const (
	Network = "10.44.221.0/24"
	Machine = "10.44.221.1"
	Port    = 51820
)

// Peer is one device that may come in through the tunnel.
type Peer struct {
	// Device ties this to a paired device, so unpairing one can take its way
	// in with it.
	Device string `json:"device"`

	Name      string    `json:"name"`
	Address   string    `json:"address"`
	PublicKey string    `json:"public_key"`
	Added     time.Time `json:"added"`

	/*
	 * Private is the device's own key, kept only until it has been collected.
	 *
	 * WireGuard has no way to hand a key to a phone except by giving it the
	 * key, and a phone cannot make one and register it without an app that
	 * does not exist. So it is made here, held until the device has taken it,
	 * and then forgotten — which is why collecting it is a thing that happens
	 * once, on the home network, in person.
	 */
	Private string `json:"private,omitempty"`
}

// Tunnel is this machine's side of it.
type Tunnel struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`

	/*
	 * Endpoint is where the house is, from outside it.
	 *
	 * Typed by its owner rather than looked up, because looking it up means
	 * asking a service on the internet what somebody's home address is, which
	 * is precisely the kind of small favour this program does not do. A name
	 * from a dynamic DNS service is the usual answer; an address works until
	 * the line is renumbered.
	 */
	Endpoint string `json:"endpoint"`

	Port  int    `json:"port"`
	Peers []Peer `json:"peers"`
}

// Load reads the tunnel for a brain, or an empty one when there is none.
func Load(root string) (*Tunnel, error) {
	t := &Tunnel{Port: Port}

	raw, err := os.ReadFile(filepath.Join(root, FileName))

	if os.IsNotExist(err) {
		return t, nil
	}

	if err != nil {
		return t, fmt.Errorf("reading %s: %w", FileName, err)
	}

	if err := json.Unmarshal(raw, t); err != nil {
		return t, fmt.Errorf("reading %s: %w", FileName, err)
	}

	if t.Port == 0 {
		t.Port = Port
	}

	return t, nil
}

// Save writes it back, whole and moved into place.
func (t *Tunnel) Save(root string) error {
	raw, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}

	temp := filepath.Join(root, FileName+".new")

	// Readable only by its owner: this file holds the key to the house.
	if err := os.WriteFile(temp, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", FileName, err)
	}

	return os.Rename(temp, filepath.Join(root, FileName))
}

// Started reports whether this machine has a tunnel at all yet.
func (t *Tunnel) Started() bool { return t.PrivateKey != "" }

// Start makes this machine's own key, once.
func (t *Tunnel) Start() error {
	if t.Started() {
		return nil
	}

	private, public, err := Keys()
	if err != nil {
		return err
	}

	t.PrivateKey, t.PublicKey, t.Port = private, public, Port

	return nil
}

/*
 * Keys makes a WireGuard key pair.
 *
 * X25519 from the standard library, which is the same curve WireGuard uses —
 * so a key made here is a key wg accepts, and no part of this needs a tool to
 * be installed before somebody can be shown what their tunnel will look like.
 */
func Keys() (private, public string, err error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("making a key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(key.Bytes()),
		base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}

/*
 * Add gives a device its own way in.
 *
 * One address each, from the top down, skipping anything already taken — so a
 * device removed and added again does not inherit the address of the one
 * before it, which would let an old configuration somebody forgot to delete
 * carry on working as somebody else.
 */
func (t *Tunnel) Add(device, name string) (Peer, error) {
	if !t.Started() {
		return Peer{}, fmt.Errorf("the tunnel has not been set up yet")
	}

	for _, existing := range t.Peers {
		if existing.Device == device {
			return existing, nil
		}
	}

	address, err := t.freeAddress()
	if err != nil {
		return Peer{}, err
	}

	private, public, err := Keys()
	if err != nil {
		return Peer{}, err
	}

	peer := Peer{
		Device:    device,
		Name:      name,
		Address:   address,
		PublicKey: public,
		Private:   private,
		Added:     time.Now().UTC(),
	}

	t.Peers = append(t.Peers, peer)

	return peer, nil
}

func (t *Tunnel) freeAddress() (string, error) {
	taken := map[string]bool{Machine: true}

	for _, p := range t.Peers {
		taken[p.Address] = true
	}

	base := net.ParseIP(Machine).To4()
	if base == nil {
		return "", fmt.Errorf("the tunnel's own address is not readable")
	}

	for i := 2; i < 255; i++ {
		candidate := fmt.Sprintf("%d.%d.%d.%d", base[0], base[1], base[2], i)

		if !taken[candidate] {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("the tunnel is full, which takes two hundred and fifty devices")
}

// Collected forgets a device's key once it has been taken, so the only copy is
// on the device it belongs to.
func (t *Tunnel) Collected(device string) {
	for i := range t.Peers {
		if t.Peers[i].Device == device {
			t.Peers[i].Private = ""
		}
	}
}

// Remove takes a device's way in away. Called when a device is unpaired, so
// losing a phone does not leave a key to the house on it.
func (t *Tunnel) Remove(device string) bool {
	kept := t.Peers[:0]
	removed := false

	for _, p := range t.Peers {
		if p.Device == device {
			removed = true

			continue
		}

		kept = append(kept, p)
	}

	t.Peers = kept

	return removed
}

// Peer finds one by device.
func (t *Tunnel) Peer(device string) (Peer, bool) {
	for _, p := range t.Peers {
		if p.Device == device {
			return p, true
		}
	}

	return Peer{}, false
}

/*
 * MachineConfig is this end of the tunnel, as wg-quick reads it.
 *
 * The address and the peers, and nothing about routing: this end answers, it
 * does not send anybody's traffic anywhere. A phone connected to it reaches
 * this machine and nothing else on the house network, which is the smallest
 * thing that does the job.
 */
func (t *Tunnel) MachineConfig() string {
	var b strings.Builder

	b.WriteString("# The assistant's end of the tunnel. Written by the program.\n")
	b.WriteString("# Bring it up with: wg-quick up " + Interface + "\n\n")
	b.WriteString("[Interface]\n")
	b.WriteString("Address = " + Machine + "/24\n")
	b.WriteString(fmt.Sprintf("ListenPort = %d\n", t.Port))
	b.WriteString("PrivateKey = " + t.PrivateKey + "\n")

	for _, p := range t.Peers {
		b.WriteString("\n# " + p.Name + "\n")
		b.WriteString("[Peer]\n")
		b.WriteString("PublicKey = " + p.PublicKey + "\n")
		b.WriteString("AllowedIPs = " + p.Address + "/32\n")
	}

	return b.String()
}

/*
 * PeerConfig is the device's end, which is what gets carried to the phone.
 *
 * AllowedIPs is this machine alone, not everything. A tunnel that captured all
 * of a phone's traffic would route somebody's whole internet through their
 * house — slower, and a far larger promise than "reach my assistant".
 */
func (t *Tunnel) PeerConfig(p Peer, port string) string {
	var b strings.Builder

	b.WriteString("# " + p.Name + "'s way in to the assistant.\n")
	b.WriteString("# Import this into WireGuard on the device.\n\n")
	b.WriteString("[Interface]\n")
	b.WriteString("Address = " + p.Address + "/32\n")
	b.WriteString("PrivateKey = " + p.Private + "\n\n")
	b.WriteString("[Peer]\n")
	b.WriteString("PublicKey = " + t.PublicKey + "\n")
	b.WriteString("AllowedIPs = " + Machine + "/32\n")
	b.WriteString(fmt.Sprintf("Endpoint = %s:%d\n", t.Endpoint, t.Port))

	/*
	 * A keepalive, because the house is behind a router.
	 *
	 * Without it the router forgets the way back within a minute or two of
	 * quiet, and the assistant becomes unreachable until the phone happens to
	 * speak first — which presents as it working sometimes.
	 */
	b.WriteString("PersistentKeepalive = 25\n")

	if port != "" {
		b.WriteString("\n# Then open: https://" + Machine + ":" + port + "\n")
	}

	return b.String()
}

// Ready says what is still missing before the tunnel can carry anything, in
// the owner's terms.
func (t *Tunnel) Ready() string {
	switch {
	case !t.Started():
		return "the tunnel has not been set up yet"
	case strings.TrimSpace(t.Endpoint) == "":
		return "it does not know how to find your house from outside it"
	case len(t.Peers) == 0:
		return "no device has been given a way in yet"
	default:
		return ""
	}
}
