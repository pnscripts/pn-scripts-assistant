---
name: web-development-basics
description: Help build or fix a website or small web app with HTML, CSS and JavaScript - page structure, responsive layout, forms, fetching data, browser developer tools, performance, basic security and deployment to static hosting. Use when the user asks how to start a website, why a layout breaks on mobile, why a button or script does not work, how to call an API from the browser, what a console or CORS error means, or how to publish a simple site.
---

# Web development basics

Start with plain, semantic HTML that works on its own, style it with CSS, and add JavaScript only for behaviour. Debug in the browser's developer tools, not by guessing.

## Steps
1. **Clarify the goal and stack.** Static page, site with a CMS, or app? Any framework already in use? Who edits the content? Keep beginners on plain HTML/CSS/JS or a static-site generator until they need more.
2. **Structure with semantic HTML.** One `<h1>`, then headings in order; `<header>`, `<nav>`, `<main>`, `<footer>`; `<button>` for actions, `<a href>` for navigation; `<label for>` on every input; `alt` text on meaningful images.
3. **Lay out with modern CSS.** Flexbox for rows and columns of items, Grid for page layouts; mobile-first styles with `min-width` media queries; relative units (`rem`, `%`, `fr`), `max-width` on text (about 60–75 characters).
4. **Add behaviour with JavaScript.** Load with `<script type="module" src="app.js"></script>` or `defer`; attach listeners with `addEventListener`; never put secrets (API keys with write access) in front-end code.
5. **Fetch data safely.**
   ```js
   async function loadItems() {
     const res = await fetch("/api/items");
     if (!res.ok) throw new Error(`HTTP ${res.status}`);
     return res.json();
   }
   ```
   Show loading and error states; insert user content with `textContent`, not `innerHTML`.
6. **Debug with developer tools (F12).** Console for errors and line numbers; Elements to inspect computed styles and box sizes; Network for status codes, request bodies and CORS headers; device toolbar for phone sizes.
7. **Check quality.** Run Lighthouse in the browser for performance, accessibility, SEO and best practices; compress images (WebP or AVIF, correct dimensions, `loading="lazy"` below the fold); validate HTML.
8. **Deploy.** Static hosting (many offer free tiers) or the user's host via Git or SFTP; HTTPS always; custom domain via the DNS records the host gives.

## Minimal page template
```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Page title</title>
  <meta name="description" content="One-sentence summary for search results">
  <link rel="stylesheet" href="styles.css">
</head>
<body>
  <header><nav><a href="/">Home</a></nav></header>
  <main><h1>Main heading</h1></main>
  <footer>Contact: name@example.com</footer>
  <script type="module" src="app.js"></script>
</body>
</html>
```

## Common errors
- `Uncaught TypeError: Cannot read properties of null` - the script ran before the element existed, or the selector is wrong; use `defer` and check the id.
- CORS error - the API server must send `Access-Control-Allow-Origin` for your site; fix on the server or call it through your own backend, never by disabling browser security.
- Layout fine on desktop, broken on phone - missing viewport meta tag, fixed widths in px, or an image wider than the screen (`img { max-width: 100%; height: auto; }`).
- 404 on CSS or JS after deploy - wrong relative path or letter case (servers are often case-sensitive).

## Pitfalls
- Copying large frameworks for a one-page site adds weight and maintenance.
- `<div onclick>` instead of `<button>` breaks keyboard and screen-reader use.
- Forms need server-side validation too; client-side checks are only for convenience.
- Contact forms without spam protection (honeypot field, rate limit) get flooded.

## Output
Hand back working code or the exact change (file and lines), the reason for the bug if there was one, how to test it in developer tools, and the next step for deployment if asked.
