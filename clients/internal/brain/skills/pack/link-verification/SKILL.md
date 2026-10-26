---
name: link-verification
description: Check whether a URL, short link or QR code is safe and really belongs to who it claims, without opening it - read the real domain, spot look-alikes, expand short links safely, check reputation, and decide whether to visit. Use when the user pastes a link and asks "is this link safe", "is this the real site", "should I open this", gets a link by SMS, email, chat or QR code, or sees a shortened link (bit.ly-style) or an odd domain.
---

# Link verification

Never open, click or prefetch the link while checking it. Work from the text of the URL. The safest path to any important site (bank, tax office, courier, email) is to type the known address by hand or use the official app, whatever the link says.

## Steps
1. **Get the exact URL as text.** Ask the user to copy it (long-press or right-click, "Copy link") rather than open it. For a QR code, use a scanner that shows the URL before opening.
2. **Find the real domain.** Take the part between `://` and the first single `/`. The registered domain is the last two labels (or three for country forms like `co.uk`). Everything to the left is a subdomain anyone can choose.
   - `https://login.example.com.account-check.net/x` - real domain is `account-check.net`, not `example.com`.
   - `https://example.com@203.0.113.7/` - the part before `@` is ignored; the real host is the IP address.
3. **Look for look-alikes:** swapped or added letters (`examp1e.com`, `exarnple.com`), hyphens and extra words (`example-secure.com`), unusual endings (`.example.top`), and mixed alphabets (Cyrillic `е` or `о` inside a Latin name; punycode starting `xn--`).
4. **Check the path and parameters** for tricks: another full URL inside (`?redirect=https://...`), login or payment words on an unrelated domain, file downloads (`.exe`, `.apk`, `.scr`, `.zip`, `.iso`, macro-enabled `.docm`).
5. **Expand short links safely** with a link-expander or preview feature that shows the destination without visiting it, then repeat steps 2-4 on the destination.
6. **Check reputation** of the domain with a reputable URL-scanning or safe-browsing lookup service. Clean results do not prove safety; new scam domains are often not listed yet.
7. **Compare with the official address.** Find it independently (printed on a card, contract or bill, or by typing the brand's known address). If the domain does not match exactly, treat it as unsafe.
8. **Decide and advise.**

## Verdict guide
- **Safe to use:** exact official domain, HTTPS, no redirects, expected path. Even then, prefer typing the address yourself for logins and payments.
- **Suspicious:** unknown domain, short link, mismatch with the sender, urgency in the message. Do not open; go to the official site manually.
- **Dangerous:** look-alike domain, login or payment page on a foreign domain, executable download, IP-address host. Do not open; report and delete.

## Pitfalls
- The padlock and HTTPS only mean encryption; scam sites have them too.
- The visible link text can differ from the real target; read the target.
- "Opening just to look" can still trigger downloads or tracking; do not.
- A link from a friend may come from a hacked account.

## If the user already opened it
Close the page; do not enter anything. If credentials or card data were entered, follow the recovery steps in `phishing-and-scam-check`. If a file downloaded, do not open it; delete it and run a security scan.

## Worked examples
| URL (as text) | Real domain | Verdict |
|---|---|---|
| `https://www.example.com/account` | `example.com` | safe if this is the known official domain |
| `https://example.com.secure-verify.top/login` | `secure-verify.top` | dangerous: look-alike login |
| `https://examp1e.com/pay` | `examp1e.com` | dangerous: digit 1 instead of letter l |
| `https://short.example/x7Kq` | unknown until expanded | suspicious: expand first |
| `http://198.51.100.24/invoice.zip` | IP address | dangerous: bare IP and archive download |

## Output
Hand back: the real domain, the red flags found, the verdict (safe / suspicious / dangerous), the safe alternative way to reach the service, and what to do if it was already opened.
