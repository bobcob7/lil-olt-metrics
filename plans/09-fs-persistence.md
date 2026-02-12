# ✅ COMPLETE — Plan 09 — FS Persistence, WAL, Compaction & Retention

## Summary

Add durable storage to the in-memory store: a write-ahead log (WAL) for crash recovery, periodic persistence of head-block data to on-disk blocks, compaction of small blocks into larger ones, and time/size-based retention to bound disk usage. After this plan, the server survives restarts without losing data.

## Dependencies

- **Plan 04** — In-memory Store (head block to be persisted)
- **Plan 08** — Server wiring (startup sequence needs WAL replay; shutdown needs flush)

## Scope

### In Scope

- Write-ahead log (WAL) in `internal/store/`:
  - Append-only binary log of incoming samples
  - Segment rotation when a segment reaches a size threshold
  - Replay on startup to rebuild the head block
  - Truncation of WAL segments that have been persisted to blocks
- Persistent blocks:
  - Periodically flush head-block data older than a threshold into an immutable on-disk block
  - Block format: one directory per block with an index file, chunk files, and a metadata file
  - Blocks are immutable once written
- Compaction:
  - Merge multiple small blocks into fewer larger blocks
  - Run as a background goroutine on a configurable interval
  - Overlapping time ranges are merged correctly
- Retention:
  - Time-based: delete blocks whose max timestamp is older than the retention duration
  - Size-based: delete oldest blocks when total disk usage exceeds the configured limit
  - Retention runs after each compaction cycle
- Updated server startup sequence: replay WAL → open existing blocks → resume normal operation
- Updated graceful shutdown: flush head block to WAL, close WAL cleanly
- FS config integration: data directory path, WAL segment size, compaction interval, retention settings from Plan 02 config
- Unit tests covering: WAL write and replay, block persistence and read-back, compaction merging, retention pruning, crash recovery simulation (write WAL, kill, replay)

### Out of Scope

- Chunk encoding optimization (XOR/delta) — use simple encoding for now
- Concurrent block reads with memory-mapped files
- Remote replication of blocks
- Block-level deduplication

## Acceptance Criteria

1. Samples written to the Store are recorded in the WAL
2. On startup, the WAL is replayed and all persisted samples are available for query
3. Head-block data older than the flush threshold is persisted to an on-disk block
4. On-disk blocks are readable by the Store and included in query results
5. Compaction merges small blocks and the merged data is queryable
6. Time-based retention deletes blocks older than the configured duration
7. Size-based retention deletes oldest blocks when disk usage exceeds the limit
8. Crash recovery: write samples, simulate crash (no clean shutdown), restart, verify all WAL-logged samples are recovered
9. Clean shutdown: WAL is flushed and closed without data loss
10. All background goroutines (compaction, retention) shut down cleanly on server stop
11. All tests use `t.Parallel()`, use temporary directories, and clean up after themselves

## Key Decisions

- **Custom WAL over Prometheus TSDB WAL**: Prometheus TSDB's WAL is tightly coupled to its internal structures; a simple custom WAL (append records, segment rotation, sequential replay) is easier to understand and maintain for this project's scale
- **Simple block format**: Directory-per-block with a flat index file; no need for Prometheus's complex chunk encoding at this scale
- **Compaction as background goroutine**: Runs periodically rather than on every write; configurable interval balances disk usage against CPU cost
- **Retention after compaction**: Ensures retention decisions are made against compacted (accurate size) blocks, not pre-compaction fragments
- **Flush threshold by age, not count**: Flushing data older than N minutes (configurable) provides predictable memory usage regardless of write rate
