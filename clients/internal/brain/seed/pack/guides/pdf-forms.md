# Filling in PDF forms

Open **PDF forms** in the menu, or ask: "fill in this application with my
details", "what fields does this form have?".

## Open the form
Give the full path of a fillable PDF form. Its fields are laid out by kind:
text fields, checkboxes, radio buttons and lists, with what each holds now,
its choices, and whether it is required or read-only. In the chat, the
assistant reads the same list and fills the fields by their names.

## Saved as a new file, checked
The filled form is saved as a new file beside the original — "(filled)"
after its name — and the original is never changed. A file that is already
there is never written over. Before it is saved, the new file is read again
and every value checked against what was asked for; a value a field cannot
take (too long, not one of the choices, a read-only field) is said, and the
rest is still filled.

## Any language
Every filled value is drawn into the form, so it shows the same in every
PDF reader: in Helvetica when it is plain Latin text, as most forms expect,
and otherwise in a font from this computer that has the letters — Cyrillic
included — embedded with only the letters used, so the file grows by a few
kilobytes. The text can still be copied and searched.

## What it leaves alone
An encrypted PDF is not opened, and a signed one is not filled, because
filling it would break the signature. Signature fields are signed in a PDF
program. Nothing leaves this computer.
