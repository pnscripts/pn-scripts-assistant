<?php

namespace App\Brain\Memory;

use Illuminate\Support\Facades\DB;

/**
 * The shape of what the brain knows, as nodes and the links between them.
 *
 * Built from the embeddings that already exist rather than from anything
 * decorative. Two facts are joined because the brain genuinely considers them
 * related — the same similarity that drives recall — so the picture is a view
 * of memory rather than a graphic that happens to sit near it. When a cluster
 * looks dense, that is because those memories really do cluster.
 */
class MemoryMap
{
    /** How many neighbours each node may link to. More becomes a hairball. */
    private const LINKS_PER_NODE = 3;

    /** Below this, a link says nothing; everything is faintly like everything. */
    private const MIN_SIMILARITY = 0.55;

    /**
     * Capped because this is drawn every frame on a canvas. Past a few hundred
     * nodes the picture stops being readable and starts costing battery.
     */
    private const MAX_NODES = 220;

    public function build(): array
    {
        $facts = DB::table('knowledge_facts')
            ->select('id', 'category', 'content')
            ->whereNotNull('embedding')
            ->orderByDesc('id')
            ->limit(self::MAX_NODES)
            ->get();

        if ($facts->isEmpty()) {
            return ['nodes' => [], 'links' => [], 'total' => 0];
        }

        return [
            'nodes' => $facts->map(fn ($f) => [
                'id' => $f->id,
                'category' => $f->category ?? 'unknown',
                // Enough to recognise a node on hover, not enough to render an
                // essay into a canvas label.
                'label' => str($f->content)->limit(60)->toString(),
            ])->values()->all(),
            'links' => $this->links($facts->pluck('id')->all()),
            'total' => DB::table('knowledge_facts')->count(),
        ];
    }

    /**
     * Nearest neighbours per node, from the vectors themselves.
     *
     * One query with a lateral join rather than a query per node: at a couple
     * of hundred facts the round trips would dominate, and this runs whenever
     * the interface opens.
     */
    private function links(array $ids): array
    {
        if ($ids === []) {
            return [];
        }

        if (DB::getDriverName() !== 'pgsql') {
            // SQLite has no vector operator; the map degrades to clusters
            // without links rather than failing.
            return [];
        }

        $rows = DB::select(
            'SELECT a.id AS source, n.id AS target, 1 - (a.embedding <=> n.embedding) AS strength
             FROM knowledge_facts a
             CROSS JOIN LATERAL (
                 SELECT b.id, b.embedding
                 FROM knowledge_facts b
                 WHERE b.id <> a.id AND b.embedding IS NOT NULL AND b.id = ANY(?)
                 ORDER BY a.embedding <=> b.embedding
                 LIMIT ?
             ) n
             WHERE a.embedding IS NOT NULL AND a.id = ANY(?)',
            ['{'.implode(',', $ids).'}', self::LINKS_PER_NODE, '{'.implode(',', $ids).'}']
        );

        $seen = [];
        $links = [];

        foreach ($rows as $row) {
            if ($row->strength < self::MIN_SIMILARITY) {
                continue;
            }

            // A links to B and B links to A describe one edge; drawing both
            // doubles the work and darkens the line for no reason.
            $key = min($row->source, $row->target).':'.max($row->source, $row->target);

            if (isset($seen[$key])) {
                continue;
            }

            $seen[$key] = true;

            $links[] = [
                'source' => (int) $row->source,
                'target' => (int) $row->target,
                'strength' => round((float) $row->strength, 3),
            ];
        }

        return $links;
    }
}
