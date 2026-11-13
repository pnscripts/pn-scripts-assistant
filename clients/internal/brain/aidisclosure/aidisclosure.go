/*
 * Package aidisclosure marks what the assistant sends and writes as made by an AI.
 *
 * From 2 August 2026, article 50 of the EU AI Act (Regulation (EU)
 * 2024/1689) asks two things of an AI system that talks with people and
 * writes text for them. Paragraph 1: the person is told, clearly and at the
 * latest at the first interaction, that they are dealing with an AI.
 * Paragraph 2: text it generates is marked, in a machine-readable way, as
 * made by an AI.
 *
 * Paragraph 1 is met by the terms of use the person accepts before the
 * first conversation (package terms): the assistant does not keep saying it
 * is an AI, it behaves as an assistant and answers truthfully when asked.
 * Paragraph 2 is here, invisible in the words and readable by machines: a
 * sent mail carries an AI-Disclosure header (MailHeader, MailValue) and a
 * written document carries it in its properties (DocumentNote,
 * DocumentKeywords, Producer). Every new way out (another sender, another
 * document format) takes its mark from here, never as words added to text
 * its owner approved.
 */
package aidisclosure

/*
 * The mark on a sent mail.
 *
 * There is no standard header for this in mail. The AI-Disclosure field of
 * the IETF draft draft-abaris-aicdh-01 (an individual draft for HTTP, read
 * 13.11.2026) is the closest published shape, a structured dictionary, so
 * mail uses the same name and syntax: mode=ai-originated is "AI produced the
 * core content and a person reviewed it", which is what happens here, since
 * nothing is sent without its owner reading and approving the words.
 */
const (
	MailHeader = "AI-Disclosure"
	MailValue  = `mode=ai-originated; provider="PN Scripts Assistant"`
)

// The mark on a written document, in its properties (PDF Info, Word core
// properties). Plain ASCII with nothing to escape in PDF strings or XML.
const (
	Producer         = "PN Scripts Assistant"
	DocumentKeywords = "AI-generated"
	DocumentNote     = "Written with PN Scripts Assistant, an AI assistant."
)
