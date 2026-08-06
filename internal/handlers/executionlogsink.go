package handlers

import (
	"log"
	"sync"

	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/services"
)

// executionLogSink persists an execution's output chunks as the process
// produces them, so the log endpoint can serve a run that has not finished.
//
// Sequence numbers are assigned here rather than by the table's rowid because
// they must survive the rolling delete below: a client polling with
// after_seq=N has to keep getting newer chunks even after the rows it already
// read are gone.
type executionLogSink struct {
	execID     int64
	limitBytes int

	mu       sync.Mutex
	seq      int64
	retained int64
}

func newExecutionLogSink(execID int64, limitBytes int) *executionLogSink {
	return &executionLogSink{execID: execID, limitBytes: limitBytes}
}

func (s *executionLogSink) WriteChunk(stream, chunk string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	_, err := database.DB.Exec(`
		INSERT INTO execution_logs (execution_id, seq, stream, chunk)
		VALUES (?, ?, ?, ?)
	`, s.execID, s.seq, stream, chunk)
	if err != nil {
		log.Printf("write execution log for %d: %v", s.execID, err)
		return
	}

	s.retained += int64(len(chunk))
	s.trim()
}

// trim drops the oldest chunks until the retained size is back under the cap.
// The newest chunk is never dropped: a single oversized chunk would otherwise
// wipe the log clean and leave the client with nothing at all.
func (s *executionLogSink) trim() {
	for s.limitBytes > 0 && s.retained > int64(s.limitBytes) {
		var id, seq, size int64
		err := database.DB.QueryRow(`
			SELECT id, seq, length(CAST(chunk AS BLOB))
			FROM execution_logs WHERE execution_id = ? ORDER BY seq LIMIT 1
		`, s.execID).Scan(&id, &seq, &size)
		if err != nil || seq == s.seq {
			return
		}
		if _, err := database.DB.Exec("DELETE FROM execution_logs WHERE id = ?", id); err != nil {
			return
		}
		s.retained -= size
	}
}

// sinkFor returns a sink for a real execution row, or a nil interface when
// the execution row failed to insert and there is nothing to attach logs to;
// services.Executor treats a nil sink as "aggregate only". Returning the
// untyped nil here is the point — handing back a nil *executionLogSink would
// produce a non-nil interface and the executor would call through it.
func sinkFor(execID int64, limitBytes int) services.LogSink {
	if execID == 0 {
		return nil
	}
	return newExecutionLogSink(execID, limitBytes)
}
