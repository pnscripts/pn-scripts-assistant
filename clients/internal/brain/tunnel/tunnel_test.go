package tunnel

import (
	"encoding/base64"
	"strings"
	"testing"
)

/*
 * The keys are the ones WireGuard uses.
 *
 * X25519 from the standard library, which is the same curve — so a key made
 * here is a key wg accepts, and nothing has to be installed before somebody
 * can be shown what their tunnel will look like.
 */
func TestTheKeysAreTheOnesWireGuardUses(t *testing.T) {
	private, public, err := Keys()
	if err != nil {
		t.Fatal(err)
	}

	for what, key := range map[string]string{"private": private, "public": public} {
		raw, err := base64.StdEncoding.DecodeString(key)
		if err != nil {
			t.Fatalf("the %s key is not base64: %v", what, err)
		}

		if len(raw) != 32 {
			t.Errorf("the %s key is %d bytes, and WireGuard's are 32", what, len(raw))
		}
	}

	if private == public {
		t.Fatal("the private and public keys are the same")
	}

	// And two of them differ, which is the whole of a key being a key.
	other, _, _ := Keys()

	if other == private {
		t.Error("two keys made in a row are the same")
	}
}

/*
 * Every device gets its own address, and a removed one does not hand its
 * address on.
 *
 * Otherwise a configuration somebody forgot to delete from an old phone
 * carries on working as whoever took that address next.
 */
func TestEachDeviceGetsItsOwnAddress(t *testing.T) {
	tun := &Tunnel{}

	if err := tun.Start(); err != nil {
		t.Fatal(err)
	}

	phone, err := tun.Add("d1", "phone")
	if err != nil {
		t.Fatal(err)
	}

	laptop, _ := tun.Add("d2", "laptop")

	if phone.Address == laptop.Address {
		t.Fatalf("two devices share %s", phone.Address)
	}

	if phone.Address == Machine || laptop.Address == Machine {
		t.Error("a device was given the machine's own address")
	}

	// Asking again for a device that already has one changes nothing.
	again, _ := tun.Add("d1", "phone")

	if again.Address != phone.Address {
		t.Errorf("the phone's address moved from %s to %s", phone.Address, again.Address)
	}

	if len(tun.Peers) != 2 {
		t.Errorf("%d peers, want 2", len(tun.Peers))
	}
}

/*
 * A device's key is kept only until it has been collected.
 *
 * WireGuard has no way to hand a key to a phone except by giving it the key,
 * so it is made here and forgotten the moment it has been taken — which is why
 * collecting it happens once, at home.
 */
func TestAKeyIsForgottenOnceItHasBeenTaken(t *testing.T) {
	tun := &Tunnel{}
	tun.Start()

	peer, _ := tun.Add("d1", "phone")

	if peer.Private == "" {
		t.Fatal("the device was given no key, so it cannot connect")
	}

	tun.Collected("d1")

	kept, _ := tun.Peer("d1")

	if kept.Private != "" {
		t.Error("the key is still here after the device took it")
	}

	// Its way in still works: the public half is what this end needs.
	if !strings.Contains(tun.MachineConfig(), kept.PublicKey) {
		t.Error("the device is no longer in this machine's configuration")
	}
}

/*
 * Unpairing a device takes its way in with it.
 *
 * A phone left in a taxi must not still hold a key to the house.
 */
func TestUnpairingTakesTheWayInWithIt(t *testing.T) {
	tun := &Tunnel{}
	tun.Start()
	tun.Add("d1", "phone")

	peer, _ := tun.Peer("d1")

	if !tun.Remove("d1") {
		t.Fatal("nothing was removed")
	}

	if _, still := tun.Peer("d1"); still {
		t.Error("the device is still in the tunnel")
	}

	if strings.Contains(tun.MachineConfig(), peer.PublicKey) {
		t.Error("this machine still lets that key in")
	}
}

/*
 * The device's tunnel carries traffic to this machine and nothing else.
 *
 * AllowedIPs of everything would route somebody's whole internet through their
 * house — slower, and a far larger promise than "reach my assistant".
 */
func TestTheTunnelCarriesOnlyWhatItIsFor(t *testing.T) {
	tun := &Tunnel{Endpoint: "home.example.net"}
	tun.Start()

	peer, _ := tun.Add("d1", "phone")

	config := tun.PeerConfig(peer, "8791")

	if !strings.Contains(config, "AllowedIPs = "+Machine+"/32") {
		t.Errorf("the tunnel carries more than it is for:\n%s", config)
	}

	if strings.Contains(config, "0.0.0.0/0") {
		t.Error("the tunnel would carry the whole of the device's internet")
	}

	// A router forgets the way back within a minute or two of quiet, which
	// presents as the assistant working sometimes.
	if !strings.Contains(config, "PersistentKeepalive") {
		t.Error("nothing keeps the way back open")
	}

	if !strings.Contains(config, "Endpoint = home.example.net:51820") {
		t.Errorf("the device does not know where to find the house:\n%s", config)
	}

	// And it says where to go once connected.
	if !strings.Contains(config, "https://"+Machine+":8791") {
		t.Errorf("it does not say what to open:\n%s", config)
	}
}

// It says what is still missing, in the owner's terms rather than by failing
// at the far end of a command.
func TestItSaysWhatIsStillMissing(t *testing.T) {
	tun := &Tunnel{}

	if why := tun.Ready(); !strings.Contains(why, "not been set up") {
		t.Errorf("with nothing done it says %q", why)
	}

	tun.Start()

	if why := tun.Ready(); !strings.Contains(why, "find your house") {
		t.Errorf("with no address it says %q", why)
	}

	tun.Endpoint = "home.example.net"

	if why := tun.Ready(); !strings.Contains(why, "no device") {
		t.Errorf("with no devices it says %q", why)
	}

	tun.Add("d1", "phone")

	if why := tun.Ready(); why != "" {
		t.Errorf("with everything done it says %q", why)
	}
}
