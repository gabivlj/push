package main

type writerN struct {
	n int64
}

func (w *writerN) Write(b []byte) (int, error) {
	w.n += int64(len(b))
	return len(b), nil
}
