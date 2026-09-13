package pair

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

/*
 * A certificate this machine makes for itself.
 *
 * Signed by nobody, because there is nobody to sign it: this is a program on
 * somebody's computer answering on their own network, and there is no name to
 * register and no authority that could vouch for 192.168.1.4. So the browser
 * will warn, once, and that warning is honest — it means "nobody has vouched
 * for this", which is true.
 *
 * What replaces the authority is the fingerprint. It is shown on the computer
 * the brain runs on, and it is shown in the warning the browser gives. If they
 * match, the thing being talked to is this program and not something between.
 * That is the same check the warning is asking somebody to make, made possible.
 *
 * The alternative — sending tokens in the clear because a warning is awkward —
 * is not a trade worth making. Anything on the network would be able to read
 * the token, and then the pairing was theatre.
 */

const (
	certificateFile = "network.crt"
	keyFile         = "network.key"
)

// HowLongACertificateLasts is long, because renewing means every device warns
// again. Ten years on a key that never leaves one machine is a reasonable
// trade for not teaching somebody to click through warnings.
const HowLongACertificateLasts = 10 * 365 * 24 * time.Hour

/*
 * Certificate returns this machine's certificate, making one if there is none.
 *
 * Remade when the addresses have changed, because a certificate that does not
 * name the address somebody is typing produces a different and much more
 * alarming warning than the one they were told to expect.
 */
func Certificate(root string) (*tls.Certificate, error) {
	names := whatThisMachineIsCalled()

	if existing, err := load(root); err == nil && covers(existing, names) {
		return existing, nil
	}

	return makeOne(root, names)
}

func load(root string) (*tls.Certificate, error) {
	loaded, err := tls.LoadX509KeyPair(
		filepath.Join(root, certificateFile), filepath.Join(root, keyFile))
	if err != nil {
		return nil, err
	}

	parsed, err := x509.ParseCertificate(loaded.Certificate[0])
	if err != nil {
		return nil, err
	}

	if time.Now().After(parsed.NotAfter) {
		return nil, fmt.Errorf("the certificate has expired")
	}

	loaded.Leaf = parsed

	return &loaded, nil
}

// covers reports whether a certificate already names every address this
// machine answers on.
func covers(certificate *tls.Certificate, names []string) bool {
	if certificate.Leaf == nil {
		return false
	}

	has := map[string]bool{}

	for _, ip := range certificate.Leaf.IPAddresses {
		has[ip.String()] = true
	}

	for _, name := range certificate.Leaf.DNSNames {
		has[name] = true
	}

	for _, name := range names {
		if !has[name] {
			return false
		}
	}

	return true
}

func makeOne(root string, names []string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("making a key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "PN Scripts Assistant"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(HowLongACertificateLasts),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},

		// Its own authority, because there is no other. See the note above.
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	for _, name := range names {
		if ip := net.ParseIP(name); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)

			continue
		}

		template.DNSNames = append(template.DNSNames, name)
	}

	body, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("making a certificate: %w", err)
	}

	encodedKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	// The key is readable only by its owner, and written before the
	// certificate: a certificate with no key beside it is a startup that fails
	// with a confusing error, where a key with no certificate is simply remade.
	err = os.WriteFile(filepath.Join(root, keyFile),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: encodedKey}), 0o600)
	if err != nil {
		return nil, fmt.Errorf("writing the key: %w", err)
	}

	err = os.WriteFile(filepath.Join(root, certificateFile),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: body}), 0o600)
	if err != nil {
		return nil, fmt.Errorf("writing the certificate: %w", err)
	}

	return load(root)
}

/*
 * Fingerprint is what somebody checks the browser's warning against.
 *
 * Formatted in pairs, because it is read off one screen and compared by eye
 * with another — which is the whole point of it, and unreadable in one long
 * string.
 */
func Fingerprint(certificate *tls.Certificate) string {
	if certificate == nil || len(certificate.Certificate) == 0 {
		return ""
	}

	sum := sha256.Sum256(certificate.Certificate[0])
	raw := strings.ToUpper(hex.EncodeToString(sum[:]))

	var out strings.Builder

	for i := 0; i < len(raw); i += 2 {
		if i > 0 {
			out.WriteString(":")
		}

		out.WriteString(raw[i : i+2])
	}

	return out.String()
}

/*
 * whatThisMachineIsCalled is every address a device on the network might use
 * to reach it.
 *
 * All of them, because somebody will type whichever their phone happens to
 * show — and a certificate that does not name the address being used produces
 * a different and far more alarming warning than the one they were told to
 * expect.
 */
func whatThisMachineIsCalled() []string {
	names := []string{"127.0.0.1", "::1", "localhost"}

	if host, err := os.Hostname(); err == nil && host != "" {
		names = append(names, host, host+".local")
	}

	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return names
	}

	for _, address := range addresses {
		network, ok := address.(*net.IPNet)
		if !ok || network.IP.IsLoopback() {
			continue
		}

		names = append(names, network.IP.String())
	}

	return names
}

/*
 * NetworkPort is the port the encrypted listener uses: one above the local one.
 *
 * Here rather than in the program that binds it, because the screen that tells
 * somebody what to type into their phone has to agree with it — and when they
 * disagreed the panel said 43231 while the listener was on 43232, which is the
 * whole feature failing in a way that looks like the network being broken.
 */
func NetworkPort(local string) string {
	n, err := strconv.Atoi(local)
	if err != nil || n <= 0 || n >= 65535 {
		return local
	}

	return strconv.Itoa(n + 1)
}

// Addresses is where a device should be told to go, for the screen that tells
// somebody what to type into their phone.
func Addresses(port string) []string {
	var out []string

	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}

	for _, address := range addresses {
		network, ok := address.(*net.IPNet)
		if !ok || network.IP.IsLoopback() || network.IP.To4() == nil {
			continue
		}

		out = append(out, "https://"+network.IP.String()+":"+port)
	}

	return out
}
