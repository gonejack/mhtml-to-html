package cmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"mime"
	"net/textproto"
	"path/filepath"
	"unicode"

	"io"
	"log"
	"net/http"

	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gabriel-vasile/mimetype"
)

type MHTMLToHTML struct {
	client    http.Client
	ImagesDir string
	Verbose   bool
}

func (h *MHTMLToHTML) Run(mhts []string) (err error) {
	if len(mhts) == 0 {
		mhts, _ = filepath.Glob("*.mht")
	}
	if len(mhts) == 0 {
		return errors.New("no mht files given")
	}

	err = h.mkdir()
	if err != nil {
		return
	}

	for _, mht := range mhts {
		if h.Verbose {
			log.Printf("processing %s", mht)
		}

		err = h.processMHTML(mht)
		if err != nil {
			return fmt.Errorf("parse %s failed: %s", mht, err)
		}
	}

	return
}
func (h *MHTMLToHTML) processMHTML(path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return
	}

	tp := textproto.NewReader(bufio.NewReader(&trimReader{rd: file}))

	// Parse the main headers
	headers, err := tp.ReadMIMEHeader()
	if err != nil {
		return
	}

	body := tp.R
	ps, err := parseMIMEParts(headers, body)
	if err != nil {
		return
	}

	var r2p = make(map[string]*part)
	var html *part
	for _, p := range ps {
		if ct := p.header.Get("Content-Type"); ct == "" {
			return ErrMissingContentType
		}
		ct, _, err := mime.ParseMediaType(p.header.Get("Content-Type"))
		if err != nil {
			return err
		}

		if html == nil && ct == "text/html" {
			html = p
			continue
		}

		ref := p.header.Get("Content-Location")
		if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
			r2p[ref] = p
		}
	}

	if html == nil {
		return errors.New("html not found")
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html.body))
	if err != nil {
		return
	}

	doc.Find("img").Each(func(i int, img *goquery.Selection) {
		h.changImgRef(img, r2p)
	})

	data, err := doc.Html()
	target := strings.TrimSuffix(path, filepath.Ext(path)) + ".html"

	return ioutil.WriteFile(target, []byte(data), 0766)
}
func (h *MHTMLToHTML) changImgRef(img *goquery.Selection, parts map[string]*part) {
	img.RemoveAttr("loading")
	img.RemoveAttr("srcset")

	src, _ := img.Attr("src")

	part, exist := parts[src]
	if !exist {
		return
	}

	if part.diskPath == "" {
		fp, err := os.CreateTemp(h.ImagesDir, "image_*")
		if err != nil {
			log.Printf("cannot create temp file for %s: %s", src, err)
			return
		}
		_, err = fp.Write(part.body)
		if err != nil {
			log.Printf("cannot write temp file for %s: %s", src, err)
			return
		}
		_ = fp.Close()

		// check mime
		fmime, err := mimetype.DetectFile(fp.Name())
		if err != nil {
			log.Printf("cannot detect image mime of %s: %s", src, err)
			return
		}
		if !strings.HasPrefix(fmime.String(), "image") {
			log.Printf("mime of %s is %s instead of images", src, fmime.String())
			return
		}

		canonical := fp.Name() + fmime.Extension()

		_ = os.Rename(fp.Name(), canonical)

		part.diskPath = canonical

		if h.Verbose {
			log.Printf("save %s as %s", src, canonical)
		}
	}

	img.SetAttr("src", part.diskPath)
}
func (h *MHTMLToHTML) mkdir() error {
	err := os.MkdirAll(h.ImagesDir, 0777)
	if err != nil {
		return fmt.Errorf("cannot make images dir %s", err)
	}

	return nil
}

// part is a copyable representation of a multipart.Part
type part struct {
	header   textproto.MIMEHeader
	body     []byte
	diskPath string
}

// trimReader is a custom io.Reader that will trim any leading
// whitespace, as this can cause email imports to fail.
type trimReader struct {
	rd      io.Reader
	trimmed bool
}

// Read trims off any unicode whitespace from the originating reader
func (tr *trimReader) Read(buf []byte) (int, error) {
	n, err := tr.rd.Read(buf)
	if err != nil {
		return n, err
	}
	if !tr.trimmed {
		t := bytes.TrimLeftFunc(buf[:n], unicode.IsSpace)
		tr.trimmed = true
		n = copy(buf, t)
	}
	return n, err
}
