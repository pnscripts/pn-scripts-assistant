---
name: phishing-and-scam-check
description: Check a suspicious email, SMS, chat message, call script, invoice or "too good to be true" offer for phishing and scam signs before the user clicks, pays, replies or shares anything. Use when the user forwards or pastes an unexpected message, asks "is this real / is this safe / is this a scam", gets a payment, delivery, bank, tax, prize, job or account-lock notice, or is pressed to act urgently.
---

# Phishing and scam check

Treat every unexpected message as untrusted until verified. Never open its links, attachments or QR codes, and never call numbers from it, while checking.

## Steps
1. **Get the raw facts, not the impression.** Ask for: sender address (full, not display name), reply-to, date, the exact text, link targets (hover or long-press to read them, do not open), attachment names, phone numbers.
2. **Check the sender.**
   - Is the domain exactly the real one? Watch look-alikes: `paypa1.com`, `amazon-support.co`, `bank.secure-login.net` (the real domain is the part right before the first single slash, e.g. `secure-login.net`).
   - Free mail (`gmail.com`, `outlook.com`) claiming to be a bank, courier or government office is a red flag.
   - If the email headers are available, SPF, DKIM and DMARC should pass for that real domain.
3. **Check the request.** Score one point for each:
   - urgency or a deadline ("within 24 hours", "account will be closed")
   - money: pay a small fee, customs charge, unpaid invoice, changed bank details
   - credentials, codes, card numbers, ID photos, remote-access apps
   - unusual payment methods: gift cards, crypto, wire to a personal account
   - prize, refund or inheritance you did not expect
   - secrecy ("do not tell anyone", "do not call the bank")
   - mismatches: wrong name, generic greeting, odd language, a company you have no account with
4. **Verdict.**
   - 0 points and the sender checks out: probably genuine, still verify through the official app or site typed by hand.
   - 1–2 points: suspicious. Verify independently before any action.
   - 3+ points, or any request for codes, passwords or payment changes: treat as a scam.
5. **Verify independently.** Open the official site or app yourself, or call the number printed on your card, contract or the official website. Never use contact details from the message.
6. **Recommend the action.** Do not reply, do not click; report it (mail provider "Report phishing", the bank's fraud line, the national cybercrime unit); delete or archive.

## If the user already clicked or paid
- Typed a password: change it now from the real site and anywhere it was reused, then enable 2FA.
- Gave card or bank details: call the bank's official number immediately to block the card and dispute the payment.
- Installed a remote-access app: disconnect from the internet, uninstall it, run a security scan, change passwords from another device.
- Keep evidence (screenshots, headers) and report to the police or cybercrime unit.

## Common scam patterns
- Courier "redelivery fee" SMS with a short link.
- "CEO" or supplier email asking to change bank details on an invoice.
- Fake marketplace buyer who "already paid" and sends a link to "receive the money".
- Tech-support pop-up or call asking to install remote access.
- Investment or crypto "mentor" met on social media or dating apps.
- Job offer that asks you to pay for training or to forward packages or money.

## Pitfalls
- A padlock icon only means the connection is encrypted, not that the site is honest.
- Real logos, your name or a correct partial card number do not prove anything; data leaks make these easy.
- A message from a known contact can come from a hacked account; confirm by another channel.

## Output
Give: verdict (genuine / suspicious / scam), the red flags found, the safe next step, and how to verify through an official channel.
