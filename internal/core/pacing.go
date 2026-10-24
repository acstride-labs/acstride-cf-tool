package core

import (
	"context"
	"github.com/gofrs/flock"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// SharedPacer serializes request starts across CLI processes and editor windows.
// OS locks release on exit; persisted time is the last start, not a future lease.
func SharedPacer(directory string, interval time.Duration) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := os.MkdirAll(directory, 0700); err != nil {
			return err
		}
		lock := flock.New(filepath.Join(directory, "requests.lock"))
		ok, err := lock.TryLockContext(ctx, 25*time.Millisecond)
		if err != nil {
			return err
		}
		if !ok {
			return ctx.Err()
		}
		defer lock.Close()
		root, err := os.OpenRoot(directory)
		if err != nil {
			return err
		}
		defer root.Close()
		if f, err := root.Open("last-start"); err == nil {
			var raw [64]byte
			n, _ := f.Read(raw[:])
			f.Close()
			last, err := strconv.ParseInt(string(raw[:n]), 10, 64)
			if err == nil {
				delay := time.Until(time.Unix(0, last).Add(interval))
				// Clock rollback cannot stall the client indefinitely.
				if delay > interval {
					delay = interval
				}
				if delay > 0 {
					timer := time.NewTimer(delay)
					defer timer.Stop()
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-timer.C:
					}
				}
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return Atomic(root, "last-start", []byte(strconv.FormatInt(time.Now().UnixNano(), 10)), 0600)
	}
}
