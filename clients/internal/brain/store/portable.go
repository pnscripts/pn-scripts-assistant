package store

import (
	"path/filepath"
	"strings"
)

/*
 * PortableSources rewrites remembered paths inside the brain at root from
 * absolute to "@brain/…" (see learning.Portable), in lessons and in facts,
 * so knowledge learned before paths were stored that way is not learned a
 * second time once the brain moves. Run at every start; after the first it
 * finds nothing to change. Reports how many rows changed.
 */
func (d *DB) PortableSources(root string) (int64, error) {
	if root == "" {
		return 0, nil
	}

	base := filepath.Clean(root) + string(filepath.Separator)
	like := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(base) + "%"

	var changed int64

	for _, table := range []string{"lessons", "knowledge_facts"} {
		for _, prefix := range []string{"document:", "reading:", "project:"} {
			res, err := d.sql().Exec(
				`UPDATE `+table+` SET source = ? || replace(substr(source, ?), '\', '/')
				 WHERE source LIKE ? ESCAPE '\'`,
				prefix+"@brain/", len(prefix)+len(base)+1, prefix+like)
			if err != nil {
				return changed, err
			}

			n, _ := res.RowsAffected()
			changed += n
		}
	}

	return changed, nil
}
