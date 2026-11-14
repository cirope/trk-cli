package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// Upload is a file to send. Open is called once, when the request is
// written, so a file is streamed from disk, not held in memory.
type Upload struct {
	Name string // The file name Trk keeps.
	Size int64
	Open func() (io.ReadCloser, error)
}

// AttachFiles adds files to card id. Trk checks every file before
// keeping any, so a refused upload keeps nothing.
func (c *Client) AttachFiles(ctx context.Context, id string, files []Upload) ([]Attachment, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	var out struct {
		Items []Attachment `json:"items"`
	}
	if err := c.sendMultipart(ctx, "/cards/"+id+"/attachments", nil, files, &out); err != nil {
		return nil, fmt.Errorf("attaching files to card %s: %w", id, err)
	}
	return out.Items, nil
}

// field is a multipart text part.
type field struct{ name, value string }

// sendMultipart POSTs fields then files as multipart/form-data, streaming
// the files, and decodes a successful JSON answer into out. It is never
// retried: a POST is not idempotent.
func (c *Client) sendMultipart(ctx context.Context, path string, fields []field, files []Upload, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.o.DownloadTimeout)
	defer cancel()
	u, err := c.url(path)
	if err != nil {
		return err
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		pw.CloseWithError(writeParts(mw, fields, files))
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), pr)
	if err != nil {
		_ = pr.Close()
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	c.authorize(req)

	resp, err := c.o.HTTPClient.Do(req)
	_ = pr.Close() // Stops the writer if the server answered early.
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	c.o.Logger.Debug("request", "method", http.MethodPost, "path", u.Path, "status", resp.StatusCode)
	data, err := c.read(resp.Body)
	if err != nil {
		return fmt.Errorf("reading the answer to POST %s: %w", path, err)
	}
	if resp.StatusCode >= 300 {
		return c.parseError(resp.StatusCode, resp.Status, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("reading the answer to POST %s: %w", path, err)
		}
	}
	return nil
}

func writeParts(mw *multipart.Writer, fields []field, files []Upload) error {
	for _, f := range fields {
		if err := mw.WriteField(f.name, f.value); err != nil {
			return err
		}
	}
	for _, f := range files {
		if err := writeFile(mw, f); err != nil {
			return err
		}
	}
	return mw.Close()
}

func writeFile(mw *multipart.Writer, f Upload) error {
	w, err := mw.CreateFormFile("files", f.Name)
	if err != nil {
		return err
	}
	r, err := f.Open()
	if err != nil {
		return fmt.Errorf("reading %s: %w", f.Name, err)
	}
	defer func() { _ = r.Close() }()
	n, err := io.Copy(w, r)
	if err != nil {
		return fmt.Errorf("reading %s: %w", f.Name, err)
	}
	if n != f.Size {
		return fmt.Errorf("%s changed while it was sent (%d bytes, not %d)", f.Name, n, f.Size)
	}
	return nil
}
