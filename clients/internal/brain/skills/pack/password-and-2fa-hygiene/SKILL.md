---
name: password-and-2fa-hygiene
description: Improve the user's account security step by step - a password manager, long unique passwords or passkeys, two-factor authentication, recovery codes, breach checks and a priority order for important accounts - without ever asking for or handling actual passwords. Use when the user asks "is my password strong", "how do I set up 2FA", "I reuse the same password", "my account was in a data breach", "should I use passkeys", "I lost my phone with the authenticator", or wants to secure email, bank or social accounts.
---

# Password and 2FA hygiene

Guide the user to strong, unique credentials and a second factor on every important account. Never ask the user to type, paste or reveal a password, code or recovery key in the chat; if they do, tell them to change it.

## Steps
1. **Prioritise accounts.** Order: primary email (it resets everything else), password manager, banking and payment, phone carrier and cloud account (Apple, Google, Microsoft), work accounts, social media, shopping, the rest.
2. **Set up a password manager** (built-in or a reputable standalone one). Protect it with a long passphrase and 2FA. Tell the user to choose based on current independent reviews; do not push a product.
3. **Replace weak and reused passwords** starting with the priority list. Let the manager generate them: at least 16 random characters, or a passphrase of 5-6 random words for the few the user must remember.
4. **Turn on 2FA** for each priority account, in this order of strength:
   - Passkey or hardware security key (phishing-resistant)
   - Authenticator app (time-based codes)
   - SMS codes (better than nothing, weaker against SIM swap)
   Menus change; tell the user to look for "Security", "Sign-in" or "Two-step verification" in the official account settings.
5. **Save recovery codes** offline (printed or in the password manager) and add a recovery email and phone that the user controls.
6. **Check for breaches** with a reputable breach-notification service, using only the email address. For each hit: change that password and anywhere it was reused.
7. **Review sign-in activity and connected apps** on the main accounts; sign out unknown sessions and remove apps no longer used.
8. **Schedule** a short review every 6 months.

## Checklist
- [ ] Password manager installed and locked with a passphrase + 2FA
- [ ] Email, bank, phone carrier and cloud account: unique password + 2FA
- [ ] Recovery codes stored offline
- [ ] No password reused across important accounts
- [ ] Breach check done; affected passwords changed
- [ ] Unknown sessions and old app connections removed

## Example passphrase pattern
`river-copper-lantern-seven-orbit` (random words chosen by a generator, not a quote or personal fact). Do not use this example itself.

## Pitfalls
- Changing passwords by following a link in a "breach" email; go to the site manually.
- Giving a 2FA code to anyone who calls or messages; real services never ask for it.
- Keeping recovery codes only on the phone that holds the authenticator.
- Security questions with true, guessable answers; treat answers like extra passwords.
- Locking the user out: always set up recovery before forcing stricter settings.

## If the authenticator phone is lost
Use recovery codes or a backup method to sign in, then remove the old device and enrol a new one. Without them, use the service's official account-recovery process.

## Output
Hand back: the prioritised account list, the steps done and still to do for each, the recovery plan, and a short 6-monthly review checklist. Never include any real password or code.
