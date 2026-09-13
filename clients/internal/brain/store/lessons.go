package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Lesson is a candidate fact held in quarantine.
type Lesson struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id,omitempty"`
	Content        string    `json:"content"`
	Status         string    `json:"status"`
	Confidence     string    `json:"confidence,omitempty"`
	Source         string    `json:"source,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// AddLesson records a proposal.
func (d *DB) AddLesson(conversationID int64, content, status, confidence, source string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := d.sql().Exec(
		`INSERT INTO lessons (conversation_id, content, status, confidence, source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		nullifyID(conversationID), content, status, nullify(confidence), nullify(source), now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("recording lesson: %w", err)
	}

	return res.LastInsertId()
}

// LessonsByStatus returns quarantined lessons at one stage.
func (d *DB) LessonsByStatus(status string, limit int) ([]Lesson, error) {
	rows, err := d.sql().Query(`
		SELECT id, COALESCE(conversation_id,0), content, status,
		       COALESCE(confidence,''), COALESCE(source,''), created_at
		FROM lessons WHERE status = ? ORDER BY id LIMIT ?`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Lesson{}

	for rows.Next() {
		var l Lesson
		var created string

		if err := rows.Scan(&l.ID, &l.ConversationID, &l.Content, &l.Status,
			&l.Confidence, &l.Source, &created); err != nil {
			return nil, err
		}

		l.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, l)
	}

	return out, rows.Err()
}

/*
 * LessonsAfter returns lessons at one stage with an id past a mark.
 *
 * For walking the whole queue rather than looking at the front of it. Taking
 * page after page of LessonsByStatus does not work here: accepting a lesson
 * removes it from the results, so the next query returns a different hundred
 * with everything shifted up — and anything that could *not* be accepted stays
 * at the front and is met again on every pass, forever. A mark that only ever
 * moves forward has neither problem, and 9,000 lessons are walked once.
 */
func (d *DB) LessonsAfter(status string, after int64, limit int) ([]Lesson, error) {
	rows, err := d.sql().Query(`
		SELECT id, COALESCE(conversation_id,0), content, status,
		       COALESCE(confidence,''), COALESCE(source,''), created_at
		FROM lessons WHERE status = ? AND id > ? ORDER BY id LIMIT ?`,
		status, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Lesson{}

	for rows.Next() {
		var l Lesson
		var created string

		if err := rows.Scan(&l.ID, &l.ConversationID, &l.Content, &l.Status,
			&l.Confidence, &l.Source, &created); err != nil {
			return nil, err
		}

		l.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, l)
	}

	return out, rows.Err()
}

/*
 * DeleteLessonsByStatus removes rows rather than moving them to rejected.
 *
 * The one operation where deleting is the point, and it needs saying why,
 * because everything else in this file is careful not to. KnownSources reads
 * this table to answer "have I already been shown this file", and it counts a
 * lesson at any status — promoted, waiting, rejected all mean seen. That is
 * right for the question it usually answers, and it means rejecting nine
 * thousand proposals does not cause a single document to be read again: the
 * rows are still there saying the file was seen.
 *
 * So "discard these and read those folders again" is two different verbs, and
 * only this one is the second half of it. Callers are expected to write the
 * rows out somewhere first — see the start-over command, which does.
 */
func (d *DB) DeleteLessonsByStatus(status string) (int64, error) {
	res, err := d.sql().Exec(`DELETE FROM lessons WHERE status = ?`, status)
	if err != nil {
		return 0, fmt.Errorf("removing %s lessons: %w", status, err)
	}

	return res.RowsAffected()
}

// SetLessonStatus moves a lesson through the pipeline.
func (d *DB) SetLessonStatus(id int64, status string) error {
	_, err := d.sql().Exec(
		`UPDATE lessons SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC().Format(time.RFC3339), id,
	)

	return err
}

// PromoteLesson stores a lesson's content as durable knowledge.
//
// The link back to the lesson is kept so a fact can always be traced to what
// proposed it — without that, a wrong fact cannot be explained, only deleted.
func (d *DB) PromoteLesson(lessonID int64, category, content string, embedding []float32) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	/*
	 * Where it came from is copied here, not left behind in the lesson.
	 *
	 * A fact outlives the lesson it was promoted from — deleting a
	 * conversation nulls that link deliberately — and a fact that cannot say
	 * where it came from can never be retired when that place is gone. Which
	 * is most of how memory stops being wrong.
	 */
	var source string

	d.sql().QueryRow(`SELECT COALESCE(source,'') FROM lessons WHERE id = ?`, lessonID).Scan(&source)

	res, err := d.sql().Exec(
		`INSERT INTO knowledge_facts
		 (promoted_from_lesson_id, category, content, embedding, dimensions, source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		lessonID, category, content, EncodeVector(embedding), len(embedding), source, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("promoting lesson %d: %w", lessonID, err)
	}

	return res.LastInsertId()
}

func nullifyID(id int64) any {
	if id == 0 {
		return nil
	}

	return id
}

// Lesson loads one by id.
func (d *DB) Lesson(id int64) (*Lesson, error) {
	var l Lesson
	var created string

	err := d.sql().QueryRow(`
		SELECT id, COALESCE(conversation_id,0), content, status,
		       COALESCE(confidence,''), COALESCE(source,''), created_at
		FROM lessons WHERE id = ?`, id).
		Scan(&l.ID, &l.ConversationID, &l.Content, &l.Status, &l.Confidence, &l.Source, &created)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	l.CreatedAt, _ = time.Parse(time.RFC3339, created)

	return &l, nil
}

/*
 * KnownSources is every file this brain has already been shown, under a folder.
 *
 * For the one question a brain that watches folders has to answer cheaply:
 * what is new here since last time. Without it, looking at a drive again means
 * embedding everything on it again — measured at two to three seconds each, so
 * a folder of two thousand documents is two hours of work to discover that
 * nothing has changed.
 *
 * Read from the lessons rather than the facts because a lesson is recorded for
 * every observation whatever became of it: promoted, waiting for a person, or
 * rejected because the file had gone. All three mean "already seen", and only
 * this table knows about the last two.
 */
/*
 * SourcesSeenAt is when each place was last read, rather than merely whether.
 *
 * KnownSources answers "have I seen this", and that answer alone meant a
 * document read in March and rewritten in June was never looked at again:
 * every source ever recorded counted as seen, at any status, forever. Memory
 * could grow and could not correct itself.
 *
 * The newest lesson from each place, because a place read twice has two and
 * the question is when it was last done.
 */
func (d *DB) SourcesSeenAt(under string) (map[string]time.Time, error) {
	rows, err := d.sql().Query(
		`SELECT source, MAX(created_at) FROM lessons
		 WHERE source IS NOT NULL AND source <> '' GROUP BY source`)
	if err != nil {
		return nil, fmt.Errorf("reading when things were last seen: %w", err)
	}

	defer rows.Close()

	seen := map[string]time.Time{}

	for rows.Next() {
		var source, at string

		if err := rows.Scan(&source, &at); err != nil {
			return nil, err
		}

		// A source is written "document:/path/to/it", so a plain prefix match
		// against the folder finds nothing. Filtered in Go for the same reason
		// KnownSources is.
		if under != "" && !strings.Contains(source, under) {
			continue
		}

		seen[source] = atTime(at)
	}

	return seen, rows.Err()
}

func (d *DB) KnownSources(under string) (map[string]bool, error) {
	rows, err := d.sql().Query(
		`SELECT DISTINCT source FROM lessons WHERE source IS NOT NULL AND source <> ''`)
	if err != nil {
		return nil, fmt.Errorf("reading what has already been seen: %w", err)
	}

	defer rows.Close()

	seen := map[string]bool{}

	for rows.Next() {
		var source string

		if err := rows.Scan(&source); err != nil {
			return nil, err
		}

		/*
		 * The path is not the whole of a source.
		 *
		 * A source is written "project:/path/to/it" or "document:/path/to/it",
		 * so asking the database for everything beginning with the folder
		 * matched nothing at all — and a folder that had just been read
		 * reported every one of its files as new, at three seconds each. The
		 * whole point of this is not doing that work twice.
		 *
		 * Filtered here rather than in the query because a LIKE with a leading
		 * wildcard cannot use an index anyway, and the wrong answer in SQL is
		 * harder to see than the right one in Go.
		 */
		if strings.HasPrefix(pathOf(source), under) {
			seen[source] = true
		}
	}

	return seen, rows.Err()
}

/*
 * CountPendingUnder is how much of one folder is waiting for a person.
 *
 * For the sentence said when a place has been read to the end. "I have
 * finished your documents" is half an answer; what somebody wants to know next
 * is how much of it they now have to look at, and the global count is the
 * wrong number when three drives are being read.
 *
 * Filtered in Go for the same reason as KnownSources: a source is written
 * "document:/path", so a LIKE on the folder matches nothing, and a LIKE with a
 * leading wildcard cannot use an index anyway.
 */
func (d *DB) CountPendingUnder(under string) (int, error) {
	rows, err := d.sql().Query(
		`SELECT source FROM lessons WHERE status = 'proposed' AND source IS NOT NULL AND source <> ''`)
	if err != nil {
		return 0, fmt.Errorf("counting what is waiting from a folder: %w", err)
	}

	defer rows.Close()

	var n int

	for rows.Next() {
		var source string

		if err := rows.Scan(&source); err != nil {
			return 0, err
		}

		if strings.HasPrefix(pathOf(source), under) {
			n++
		}
	}

	return n, rows.Err()
}

// pathOf is the file a source refers to, without the kind in front of it.
func pathOf(source string) string {
	if at := strings.Index(source, ":"); at >= 0 {
		return source[at+1:]
	}

	return source
}
