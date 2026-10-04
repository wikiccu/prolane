package record

import (
	"encoding/json/v2"
	"errors"
	"os"
	"strconv"
	"sync"
)

const (
	maxMetadataBytes = 8 << 10
	maxRecordBytes   = 64 << 10
)

type recording struct {
	mu       sync.Mutex
	file     *os.File
	nextID   uint64
	stopped  bool
	err      error
	failed   chan struct{}
	handlers sync.WaitGroup
}

func (r *recording) admit() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped || r.err != nil {
		return "", false
	}
	if r.nextID == ^uint64(0) {
		r.fail(errors.New("exchange IDs exhausted; recording is incomplete"))
		return "", false
	}
	r.nextID++
	// Admission and stopping share the mutex so Add cannot race with Wait.
	r.handlers.Add(1)
	return strconv.FormatUint(r.nextID, 10), true
}

func (r *recording) write(exchange *Exchange) {
	// ponytail: synchronous writes serialize admission and persistence; split
	// locks or add a bounded writer queue only if measured throughput needs it.
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return
	}
	data, err := json.Marshal(exchange)
	if err != nil {
		r.fail(errors.New("recording encoding failed; recording is incomplete"))
		return
	}
	if len(data)+1 > maxRecordBytes {
		r.fail(errors.New("record size limit exceeded; recording is incomplete"))
		return
	}
	data = append(data, '\n')
	if n, err := r.file.Write(data); err != nil || n != len(data) {
		r.fail(errors.New("recording write failed; recording is incomplete"))
	}
}

// fail is called with mu held, only for the first recording error.
func (r *recording) fail(err error) {
	r.err = err
	close(r.failed)
}

func (r *recording) close() error {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
	r.handlers.Wait()
	if err := r.file.Close(); err != nil {
		return errors.Join(r.err, errors.New("recording close failed; recording is incomplete"))
	}
	return r.err
}
