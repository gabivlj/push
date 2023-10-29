package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbatts/tar-split/tar/asm"
	"github.com/vbatts/tar-split/tar/storage"
)

type layerReader struct {
	sha256    hash.Hash
	tarReader io.Reader
	clean     []func()
	bytes     uint64
}

func (l layerReader) Size() uint64 {
	return l.bytes
}

func (l *layerReader) deferClean(f func()) {
	l.clean = append(l.clean, f)
}

func (l layerReader) Close() error {
	for i := len(l.clean) - 1; i >= 0; i-- {
		l.clean[i]()
	}

	return nil
}

func (l layerReader) Hash() string {
	hash := l.sha256.Sum([]byte{})
	return fmt.Sprintf("sha256:%s", string(hex.EncodeToString(hash)))
}

func (l *layerReader) Read(b []byte) (int, error) {
	n, err := l.tarReader.Read(b)
	l.sha256.Write(b[:n])
	l.bytes += uint64(n)
	return n, err
}

// TODO build chain of things
func newLayerReader(n *node) (*layerReader, error) {
	p := filepath.Join("/var", "lib", "docker", "image", storageDriver, "layerdb", "sha256", strings.Split(n.id, ":")[1])
	cacheIDPath := filepath.Join(p, "cache-id")
	cacheBytes, err := os.ReadFile(cacheIDPath)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", cacheIDPath, err)
	}

	cache := string(cacheBytes)
	pTar := filepath.Join("/var/lib/docker", storageDriver, cache, "diff")
	tarPath := filepath.Join(p, "tar-split.json.gz")
	fdJSON, err := os.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", tarPath, err)
	}

	l := &layerReader{}
	l.deferClean(func() { fdJSON.Close() })
	mfz, err := gzip.NewReader(fdJSON)
	if err != nil {
		return nil, fmt.Errorf("new reader json: %w", err)
	}

	l.deferClean(func() { mfz.Close() })
	metaUnpacker := storage.NewJSONUnpacker(mfz)
	// XXX maybe get the absolute path here
	fileGetter := storage.NewPathFileGetter(pTar)
	ots := asm.NewOutputTarStream(fileGetter, metaUnpacker)
	l.deferClean(func() { ots.Close() })
	l.tarReader = ots
	l.sha256 = sha256.New()
	return l, nil
}
