package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type pusher struct {
	manifest    *Manifest
	maxPushJobs int
}

func newPusher(manifest *Manifest, maxPushJobs int) *pusher {
	return &pusher{
		manifest, maxPushJobs,
	}
}

func (p *pusher) push(ctx context.Context, url, repository, name string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wg := &sync.WaitGroup{}
	m := p.maxPushJobs
	if len(p.manifest.Layers)+1 <= p.maxPushJobs {
		m = len(p.manifest.Layers) + 1
	}

	done := make(chan struct{}, m)
	errChann := make(chan error, m)
	jobs := make([]pushJob, 0, len(p.manifest.Layers)+1)
	for _, layer := range p.manifest.Layers {
		jobs = append(jobs, pushJob{
			layerID:    layer.Digest,
			size:       layer.Size,
			mediaType:  layer.MediaType,
			done:       done,
			errChan:    errChann,
			url:        url,
			name:       name,
			repository: repository,
		})
	}

	jobs = append(jobs, pushJob{
		layerID:    p.manifest.Config.Digest,
		mediaType:  p.manifest.Config.MediaType,
		size:       p.manifest.Config.Size,
		url:        url,
		name:       name,
		repository: repository,
		done:       done,
		errChan:    errChann,
	})

	for i := 0; i < m; i++ {
		job := jobs[len(jobs)-1]
		job.startPush(ctx, wg)
		jobs = jobs[:len(jobs)-1]
	}

forLoop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			if len(jobs) == 0 {
				break forLoop
			}

			job := jobs[len(jobs)-1]
			job.startPush(ctx, wg)
			jobs = jobs[:len(jobs)-1]
			if len(jobs) == 0 {
				break forLoop
			}
		case err := <-errChann:
			cancel()
			return fmt.Errorf("pushing layer: %w", err)
		}
	}

	allDone := make(chan struct{}, 1)
	go func() {
		wg.Wait()
		allDone <- struct{}{}
	}()

	select {
	case err := <-errChann:
		cancel()
		select {
		case <-allDone:
		case <-ctx.Done():
		}

		return fmt.Errorf("pushing at the end layer: %w", err)
	case <-allDone:
	}

	c := &http.Client{}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	encoder := json.NewEncoder(writer)
	go func() {
		if err := encoder.Encode(p.manifest); err != nil {
			fmt.Fprintln(os.Stderr, "warning encoding manifest:", err.Error())
		}
		writer.Close()
	}()

	req, err := http.NewRequest(http.MethodPut, url+path.Join("/v2", repository, "manifests", name), reader)
	if err != nil {
		return fmt.Errorf("manifest: %w", err)
	}

	req.Header.Add("Content-Type", p.manifest.MediaType)
	res, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("manifest request: %w", err)
	}

	if res.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code creating manifest: %w", err)
	}

	res.Body.Close()
	return nil
}

type pushJob struct {
	url        string
	repository string
	name       string
	layerID    string
	size       uint64
	mediaType  string
	errChan    chan<- error
	done       chan<- struct{}
}

func (p *pushJob) startPush(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := p.push(ctx)
		if err != nil {
			if errors.Is(err, ctx.Err()) {
				return
			}

			p.errChan <- fmt.Errorf("layer %q: %w", p.layerID, err)
		} else {
			fmt.Println("======> Finished layer", p.layerID)
			p.done <- struct{}{}
		}
	}()
}

func readAllBody(r io.ReadCloser) string {
	response, err := io.ReadAll(r)
	if err != nil {
		return fmt.Sprintf("(#error reading body: %v)", err)
	}

	return string(response)
}

func getRangeHeader(res *http.Response) (uint64, uint64, error) {
	r := res.Header.Get("Range")
	after, _ := strings.CutPrefix(r, "bytes=")
	s := strings.SplitN(after, "-", 2)
	startS, endS := s[0], s[1]
	start, err := strconv.ParseUint(startS, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("start %s: %w", r, err)
	}

	end, err := strconv.ParseUint(endS, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("end %s: %w", r, err)
	}

	return start, end, nil
}

func (p *pushJob) push(ctx context.Context) error {
	c := &http.Client{}
	req, err := http.NewRequest(http.MethodHead, p.url+path.Join("/v2", p.repository, "blobs", p.layerID), nil)
	if err != nil {
		return err
	}

	res, err := c.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("head: %w", err)
	}

	if res.StatusCode < 300 {
		return nil
	}

	res.Body.Close()
	req, err = http.NewRequest(http.MethodPost, p.url+path.Join("/v2", p.repository, "blobs", "uploads")+"/", nil)
	if err != nil {
		return err
	}

	res, err = c.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("post uploads: %w", err)
	}

	if res.StatusCode != http.StatusAccepted {
		return fmt.Errorf("create upload unexpected status code %v with body: %v", res.StatusCode, readAllBody(res.Body))
	}

	urlLocation, err := res.Location()
	locationForThisUpload := p.url + path.Join("/v2", p.repository, "blobs", "uploads", res.Header.Get("Docker-Upload-UUID"))
	if err == nil {
		locationForThisUpload = urlLocation.String()
	}

	_, possibleStart, err := getRangeHeader(res)
	start := uint64(0)
	if err == nil && possibleStart != 0 {
		start = possibleStart + 1
	}

	end := p.size
	fd, err := os.Open(filepath.Join(layerFolder, p.layerID))
	if err != nil {
		return fmt.Errorf("opening layer file: %w", err)
	}

	for {
		if start != 0 {
			_, err := fd.Seek(int64(start), 0)
			if err != nil {
				return fmt.Errorf("seek: %w", err)
			}
		}

		rangeHeader := fmt.Sprintf("%d-%d", start, end-1)
		contentType := "application/octet-stream"
		contentLength := fmt.Sprintf("%d", end-start)
		req, err := http.NewRequest("PATCH", locationForThisUpload, fd)
		if err != nil {
			return err
		}

		req.Header.Add("Content-Type", contentType)
		req.Header.Add("Content-Length", contentLength)
		req.Header.Add("Content-Range", rangeHeader)
		res, err := c.Do(req.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("patch upload: %w", err)
		}

		if res.StatusCode >= 300 {
			return fmt.Errorf("patch upload unexpected status code %v: %v", res.StatusCode, readAllBody(res.Body))
		}

		res.Body.Close()
		_, endRes, err := getRangeHeader(res)
		if err != nil {
			return fmt.Errorf("get range: %w", err)
		}

		urlLocation, err := res.Location()
		if err == nil {
			locationForThisUpload = urlLocation.String()
		}

		if endRes >= end-1 {
			break
		}

		start = endRes + 1
	}

	endURL, err := url.Parse(locationForThisUpload)
	if err != nil {
		return fmt.Errorf("unexpected error parsing url %q: %w", locationForThisUpload, err)
	}

	q := endURL.Query()
	q.Set("digest", p.layerID)
	endURL.RawQuery = q.Encode()
	req, err = http.NewRequest("PUT", endURL.String(), nil)
	if err != nil {
		return err
	}

	req.Header.Add("Content-Length", "0")
	res, err = c.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("creating upload: %w", err)
	}

	if res.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code creating upload %v: %v", res.StatusCode, readAllBody(res.Body))
	}

	res.Body.Close()
	return nil
}
