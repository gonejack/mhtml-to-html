package cmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/textproto"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/alecthomas/kong"
)

type options struct {
	Verbose bool     `help:"Verbose printing."`
	About   bool     `help:"Show about."`
	MHTML   []string `arg:"" optional:""`
}

type MHTMLToHTML struct {
	options
}

func (h *MHTMLToHTML) Run() (err error) {
	kong.Parse(h,
		kong.Name("mhtml-to-html"),
		kong.Description("This command line converts .mhtml file to .html file"),
		kong.UsageOnError(),
	)
	if h.About {
		fmt.Println("Visit https://github.com/gonejack/mhtml-to-html")
		return
	}
	if len(h.MHTML) == 0 {
		for _, pattern := range []string{"*.mht", "*.mhtml"} {
			found, _ := filepath.Glob(pattern)
			h.MHTML = append(h.MHTML, found...)
		}
	}
	if len(h.MHTML) == 0 {
		return errors.New("no mht files given")
	}
	for _, mht := range h.MHTML {
		if h.Verbose {
			log.Printf("processing %s", mht)
		}
		if e := h.process(mht); e != nil {
			return fmt.Errorf("parse %s failed: %s", mht, e)
		}
	}
	return
}
func (h *MHTMLToHTML) process(mht string) error {
	fd, err := os.Open(mht)
	if err != nil {
		return err
	}
	defer fd.Close()
	tp := textproto.NewReader(bufio.NewReader(&trimReader{rd: fd}))
	hdr, err := tp.ReadMIMEHeader()
	if err != nil {
		return err
	}
	parts, err := parseMIMEParts(hdr, tp.R)
	if err != nil {
		return err
	}
	var html *part
	var savedir = strings.TrimSuffix(mht, filepath.Ext(mht)) + "_files"
	var saves = make(map[string]string)
	for idx, part := range parts {
		contentType := part.header.Get("Content-Type")
		if contentType == "" {
			return ErrMissingContentType
		}
		mimetype, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return err
		}
		if html == nil && mimetype == "text/html" {
			html = part
			continue
		}

		ext := ".dat"
		switch mimetype {
		case mime.TypeByExtension(".jpg"):
			ext = ".jpg"
		default:
			exts, err := mime.ExtensionsByType(mimetype)
			if err != nil {
				return err
			}
			if len(exts) > 0 {
				ext = exts[0]
			}
		}

		dir := path.Join(savedir, mimetype)
		err = os.MkdirAll(dir, 0766)
		if err != nil {
			return fmt.Errorf("cannot create dir %s: %s", dir, err)
		}
		file := path.Join(dir, fmt.Sprintf("%d%s", idx, ext))
		err = os.WriteFile(file, part.body, 0766)
		if err != nil {
			return fmt.Errorf("cannot write file%s: %s", file, err)
		}
		ref := part.header.Get("Content-Location")
		saves[ref] = file
	}
	if html == nil {
		return errors.New("html not found")
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html.body))
	if err != nil {
		return err
	}
	doc.Find("img,link,script").Each(func(i int, e *goquery.Selection) {
		h.changeRef(e, saves)
	})
	txt, err := doc.Html()
	if err != nil {
		return err
	}
	target := strings.TrimSuffix(mht, filepath.Ext(mht)) + ".html"
	return os.WriteFile(target, []byte(txt), 0766)
}
func (h *MHTMLToHTML) changeRef(e *goquery.Selection, saves map[string]string) {
	attr := "src"
	switch e.Get(0).Data {
	case "img":
		e.RemoveAttr("loading")
		e.RemoveAttr("srcset")
	case "link":
		attr = "href"
	}
	ref, _ := e.Attr(attr)
	local, exist := saves[ref]
	if exist {
		e.SetAttr(attr, local)
	}
}

type part struct {
	header textproto.MIMEHeader
	body   []byte
}
type trimReader struct {
	rd      io.Reader
	trimmed bool
}

func (tr *trimReader) Read(buf []byte) (int, error) {
	n, err := tr.rd.Read(buf)
	if err != nil {
		return n, err
	}
	if !tr.trimmed {
		t := bytes.TrimLeftFunc(buf[:n], tr.isSpace)
		tr.trimmed = true
		n = copy(buf, t)
	}
	return n, err
}
func (tr *trimReader) isSpace(r rune) bool {
	const (
		ZWSP   = '\u200B' // ZWSP represents zero-width space.
		ZWNBSP = '\uFEFF' // ZWNBSP represents zero-width no-break space.
		ZWJ    = '\u200D' // ZWJ represents zero-width joiner.
		ZWNJ   = '\u200C' // ZWNJ represents zero-width non-joiner.
	)
	switch r {
	case ZWSP, ZWNBSP, ZWJ, ZWNJ:
		return true
	default:
		return unicode.IsSpace(r)
	}
}
