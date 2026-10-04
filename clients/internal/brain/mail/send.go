package mail

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

/*
 * Sending.
 *
 * net/smtp is in the standard library and does the whole job, so this is mostly
 * about the parts it deliberately leaves to the caller: encoding a subject that
 * is not ASCII, refusing to talk to a server that will not encrypt, and
 * checking the addresses before rather than after connecting.
 */

// Send delivers one message and returns what was sent.
func Send(a Account, to []string, subject, body string) (string, error) {
	if !a.Configured() {
		return "", errNotSetUp
	}

	if len(to) == 0 {
		return "", fmt.Errorf("say who it is for")
	}

	from := a.From
	if from == "" {
		from = a.User
	}

	// Checked here rather than discovered as a server error halfway through,
	// which arrives as a numeric code nobody can act on.
	for _, address := range append([]string{from}, to...) {
		if _, err := mail.ParseAddress(address); err != nil {
			return "", fmt.Errorf("%q is not an email address", address)
		}
	}

	host, err := sendingHost(a)
	if err != nil {
		return "", err
	}

	port := a.SMTPPort
	if port == 0 {
		port = 587
	}

	message := compose(from, to, subject, body)

	if err := deliver(host, port, a.User, a.Password, from, to, []byte(message)); err != nil {
		return "", err
	}

	return fmt.Sprintf("Sent to %s: %s", strings.Join(to, ", "), subject), nil
}

/*
 * sendingHost is the server mail goes out through.
 *
 * Left blank, it used to be the reading server, which is wrong for nearly
 * every provider: Gmail reads on imap.gmail.com and sends on smtp.gmail.com,
 * and imap.gmail.com does not answer on 587 at all. The convention those
 * names follow is common enough to rely on, so imap.<domain> becomes
 * smtp.<domain>. Any other name is not guessed at — a wrong guess sends the
 * password to a server nobody chose — and the error says what to fill in.
 */
func sendingHost(a Account) (string, error) {
	if host := strings.TrimSpace(a.SMTPHost); host != "" {
		return host, nil
	}

	host := strings.TrimSpace(a.Host)
	if len(host) > len("imap.") && strings.EqualFold(host[:len("imap.")], "imap.") {
		return "smtp." + host[len("imap."):], nil
	}

	return "", fmt.Errorf("there is no outgoing server: fill in Outgoing server (SMTP) "+
		"under Mailbox in Settings, because %s is only for reading mail", host)
}

// compose builds the message.
//
// The subject is encoded whenever it is not ASCII, because a raw non-ASCII
// header is not legal and different servers mangle it differently — and the
// owner of this brain writes in Bulgarian, so that is the ordinary case here
// rather than the exception.
func compose(from string, to []string, subject, body string) string {
	var b strings.Builder

	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: " + messageID(from) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")

	// Lone newlines are not legal line endings in a message.
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))

	return b.String()
}

/*
 * messageID names one message, as RFC 5322 section 3.6.4 asks every message to
 * be named.
 *
 * Without one, some servers add their own and some spam filters count the
 * absence against the sender. The right-hand side is the sender's own domain,
 * and the left-hand side is random, so two messages never share a name.
 */
func messageID(from string) string {
	domain := "localhost"

	if address, err := mail.ParseAddress(from); err == nil {
		if at := strings.LastIndex(address.Address, "@"); at >= 0 && at < len(address.Address)-1 {
			domain = address.Address[at+1:]
		}
	}

	random := make([]byte, 16)
	_, _ = rand.Read(random) // crypto/rand.Read does not fail on supported systems.

	return "<" + strconv.FormatInt(time.Now().UnixNano(), 36) + "." +
		hex.EncodeToString(random) + "@" + domain + ">"
}

/*
 * How long sending may take.
 *
 * Without a limit, a server that accepts the connection and then says nothing
 * holds the tool call forever. dialTimeout bounds reaching the server;
 * sessionTimeout bounds the whole conversation after that, the message
 * included. Variables rather than constants so the tests need not wait.
 */
var (
	dialTimeout    = 30 * time.Second
	sessionTimeout = 2 * time.Minute
)

/*
 * deliver talks to the submission server.
 *
 * STARTTLS is required, not attempted: if the server will not encrypt, the
 * alternative is sending somebody's password and their letter across the
 * network in plain text, and no message is worth that. On 465, which is TLS
 * from the first byte, it connects encrypted to begin with.
 */
func deliver(host string, port int, user, password, from string, to []string, message []byte) error {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	auth := smtp.PlainAuth("", user, password, host)

	conn, err := (&net.Dialer{Timeout: dialTimeout}).Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", address, err)
	}

	// One deadline for the whole session. STARTTLS wraps this connection
	// rather than replacing it, so the deadline still holds once encrypted.
	if err := conn.SetDeadline(time.Now().Add(sessionTimeout)); err != nil {
		conn.Close()

		return fmt.Errorf("connecting to %s: %w", address, err)
	}

	if port == 465 {
		conn = tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()

		return fmt.Errorf("connecting to %s: %w", address, err)
	}

	defer client.Close()

	if port != 465 {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			return fmt.Errorf("%s will not encrypt the connection, so the password "+
				"and the message would cross the network in the clear", address)
		}

		if err := client.StartTLS(&tls.Config{
			ServerName: host, MinVersion: tls.VersionTLS12,
		}); err != nil {
			return fmt.Errorf("encrypting the connection to %s: %w", address, err)
		}
	}

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("signing in to %s as %s: %w", address, user, err)
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("sending from %s: %w", from, err)
	}

	for _, address := range to {
		if err := client.Rcpt(address); err != nil {
			return fmt.Errorf("sending to %s: %w", address, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return err
	}

	if _, err := w.Write(message); err != nil {
		return err
	}

	if err := w.Close(); err != nil {
		return err
	}

	return client.Quit()
}
