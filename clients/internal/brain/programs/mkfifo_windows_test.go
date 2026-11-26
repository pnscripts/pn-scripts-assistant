package programs

import "errors"

func mkfifo(string) error { return errors.New("no pipes in folders on Windows") }
