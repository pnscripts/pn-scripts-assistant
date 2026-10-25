# Translation and your glossary

I translate texts, documents and subtitles. Open **Translation** in the menu,
or ask in the chat: "translate this into German", "translate the
offer in my Documents into English", "always translate 'invoice' as
'фактура'".

## Files keep their layout
A file is translated into a new file beside it — `offer.docx` becomes
`offer.en.docx` — and the original is never changed or replaced. Only the
words are sent to the model; everything else stays exactly as it was:

- Word (.docx) and OpenDocument (.odt): styles, tables, headers and footers,
  pictures. A paragraph's words go into its first run of formatting, so bold
  inside a sentence becomes the formatting of the whole sentence; line breaks
  and tabs stay where they were.
- Subtitles (.srt, .vtt): numbers and timings are kept; only what is said is
  translated, line by line.
- Markdown (.md): code blocks and the header block are left alone.
- Plain text (.txt): paragraph by paragraph.

A long document takes minutes on a computer without a graphics card. Ask me
to do it in the background and carry on.

## Your glossary
The glossary is your list of agreed translations: a word or phrase and how it
is always translated into a language. Write a term as itself to keep it
untranslated — a name, a brand, a product. Every translation is given the
terms it uses, and checked afterwards:

- a glossary term not translated as agreed (a Bulgarian or English ending on
  the agreed word is fine — "договора" still counts as "договор");
- a number, an e-mail address or a link that is in the original and not in
  the translation.

Anything the check finds is listed with the translation, so you can look at
it before the text goes anywhere.

Add terms one by one on the Translation page or in the chat, or many at once
by pasting two columns from a spreadsheet (term, translation; optionally from,
into, note). **Save as a spreadsheet** gives you the whole glossary as a CSV
file. The glossary is kept in your knowledge base folder as glossary.json and
travels with it.

## Privacy
Translation uses the language model you have chosen. With privacy set to
private it is a model on this computer and nothing leaves it.
