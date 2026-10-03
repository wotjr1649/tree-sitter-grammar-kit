package kit

import "errors"

func mkfifo(string) error { return errors.New("no FIFO special files on Windows") }
