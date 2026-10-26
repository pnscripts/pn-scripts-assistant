---
name: mobile-app-development
description: Help plan, build, test and release a mobile app for Android and iOS - choosing native, cross-platform (Flutter, React Native, Kotlin Multiplatform) or a web app, project setup, layouts for many screen sizes, permissions, offline data, testing on devices and preparing store releases. Use when the user wants to make an app, asks which framework to pick, hits a build, Gradle, Xcode or signing error, needs push notifications or offline storage, or prepares a Google Play or App Store submission.
---

# Mobile app development

Decide the approach from the app's needs, keep the first version small, and test on real devices early. Store rules and tool versions change often, so check the current official documentation before each release.

## Steps
1. **Pin down the scope.** Core user task in one sentence, target platforms, must-have device features (camera, GPS, Bluetooth, background work), offline needs, login, payments, and who maintains it.
2. **Choose the approach.**
   | Option | Good for | Watch out |
   |---|---|---|
   | Web app / PWA | Content, forms, simple tools | Limited background and device access on iOS |
   | Flutter or React Native | One team, both platforms, most business apps | Native modules for unusual hardware |
   | Native (Kotlin, Swift) | Heavy graphics, deep OS integration | Two codebases |
3. **Set up the project.** Official SDKs (Android Studio, Xcode on a Mac for iOS builds), version control from day one, separate debug and release configurations, app ID in reverse-domain form (`com.example.notes`).
4. **Build the UI for every size.** Flexible layouts, safe areas (notches, gesture bars), text that scales with system font size, touch targets at least 48 dp / 44 pt, dark mode, portrait and landscape if supported.
5. **Handle permissions and data.**
   - Ask for a permission only when the feature is used, with a short explanation before the system prompt; handle "denied" gracefully.
   - Store data locally (SQLite, Room, Core Data, a key-value store) and sync when online; store tokens in the Keychain or Android Keystore, never in plain preferences.
   - Call only HTTPS endpoints; keep secret keys on a server, not in the app.
6. **Test.** Unit tests for logic, UI tests for the main flows, real low-end Android phones and an older iPhone; emulators for screen sizes; test slow network, airplane mode, rotation, app killed in background.
7. **Prepare the release.**
   - Bump version name and build number every upload.
   - Sign the release build; back up the signing key and its passwords in a safe place (losing it can block updates).
   - Store listing: icon, screenshots for required sizes, short and full description, privacy policy URL, data-safety / privacy-label answers that match what the app really collects, content rating.
   - Use internal or closed testing tracks (Google Play testing, TestFlight) before production.

## Example
"Habit tracker for Android and iPhone, works offline": Flutter, local SQLite with a simple repository layer, notifications scheduled locally (no server needed), no login in version 1, export to a file for backup; release to internal testing, then a staged rollout.

## Common build errors
- Gradle "duplicate class" or SDK version errors: align library versions, update the Android Gradle Plugin as its release notes describe, clean and rebuild.
- iOS "No signing certificate" or provisioning errors: select the team in Xcode signing settings and let it manage signing automatically for development.
- App crashes only in release: code shrinking/obfuscation removed something used by reflection; add keep rules and test the release build.

## Pitfalls
- Building every feature before the first test release; ship a small core, measure, then add.
- Ignoring store policies (background location, accessibility services, subscriptions, user-generated content) leads to rejections; read the current policy pages for the features used.
- Hard-coded strings make translation painful; use string resources from the start.
- Not handling process death: Android may kill the app in the background; save state.

## Output
Hand back the recommended approach with reasons, a short architecture or file plan, the code or fix for the user's current problem, and a release checklist with the items to verify on the current official store pages.
