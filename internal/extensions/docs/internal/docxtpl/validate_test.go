// SPDX-License-Identifier: Apache-2.0

package docxtpl

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"testing"
)

func TestValidateRenderedFiles(t *testing.T) {
	files := []string{
		"../../../../../test_results/meeting-minutes-rendered.docx",
		"../../../../../test_results/ssi-rendered.docx",
	}

	for _, path := range files {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Skip("file not found")
			}
			r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatalf("not a valid ZIP: %v", err)
			}
			for _, f := range r.File {
				if len(f.Name) < 4 || f.Name[len(f.Name)-4:] != ".xml" {
					continue
				}
				rc, _ := f.Open()
				content, _ := io.ReadAll(rc)
				rc.Close()

				d := xml.NewDecoder(bytes.NewReader(content))
				for {
					_, err := d.Token()
					if err == io.EOF {
						break
					}
					if err != nil {
						pos := d.InputOffset()
						start := pos - 200
						if start < 0 {
							start = 0
						}
						end := pos + 200
						if end > int64(len(content)) {
							end = int64(len(content))
						}
						t.Errorf("%s: XML ERROR at offset %d: %v\nCONTEXT: ...%s...", f.Name, pos, err, content[start:end])
						break
					}
				}
			}
		})
	}
}
