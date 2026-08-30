package mail

import (
	"crypto/tls"
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

	host := a.SMTPHost
	if host == "" {
		host = a.Host
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
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")

	// Lone newlines are not legal line endings in a message.
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))

	return b.String()
}

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

	var (
		client *smtp.Client
		err    error
	)

	if port == 465 {
		conn, dialErr := tls.DialWithDialer(&net.Dialer{Timeout: 30 * time.Second},
			"tcp", address, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if dialErr != nil {
			return fmt.Errorf("connecting to %s: %w", address, dialErr)
		}

		client, err = smtp.NewClient(conn, host)
	} else {
		client, err = smtp.Dial(address)
	}

	if err != nil {
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
