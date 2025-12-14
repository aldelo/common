package gin

/*
 * Copyright 2020-2023 Aldelo, LP
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import (
	"fmt"
	util "github.com/aldelo/common"
	"github.com/gin-contrib/multitemplate"
	"log"
	"path/filepath"
)

// NewTemplate creates new html template render worker object
//
// templateBaseDir = (required) base directory that contains template files
// templateLayoutPath = (required) relative page layout folder path, layout defines site theme and common base pages
//  1. to use page parts, use {{ template "xyz" }}
//  2. see golang html page template rules for more information
//
// templatePagePath = (optional) relative page part folder path, pages define portion of html to be inserted into layout themes
//  1. to define page part with named template, use {{ define "xyz" }}  {{ end }}
//  2. see golang html page template rules for more information
//
// notes
//  1. c.HTML name = page html name, such as home.html, if there is no page name, then use layout name
//  2. if c.HTML name cannot find target in renderer, error 500 will be encountered
//  3. layout file should not contain page parts may not be rendered in c.HTML call
//  4. basic info about html templates = https://blog.gopheracademy.com/advent-2017/using-go-templates/
func NewTemplate(templateBaseDir string, templateLayoutPath string, templatePagePath string) *GinTemplate {
	return &GinTemplate{
		TemplateBaseDir: templateBaseDir,
		Templates: []TemplateDefinition{
			{
				LayoutPath: templateLayoutPath,
				PagePath:   templatePagePath,
			},
		},
	}
}

// GinTemplate defines the struct for working with html template renderer
type GinTemplate struct {
	TemplateBaseDir string
	Templates       []TemplateDefinition

	_htmlrenderer       multitemplate.Renderer
	_htmlTemplatesCount int
}

// TemplateDefinition defines an unit of template render target
type TemplateDefinition struct {
	LayoutPath string
	PagePath   string
}

// LoadHtmlTemplates will load html templates and set renderer into struct internal var
func (t *GinTemplate) LoadHtmlTemplates() error {
	t._htmlTemplatesCount = 0

	if util.LenTrim(t.TemplateBaseDir) == 0 {
		return fmt.Errorf("Html Template Base Dir is Required")
	}

	if len(t.Templates) == 0 {
		return fmt.Errorf("Html Template Definition is Required")
	}

	base := filepath.Clean(t.TemplateBaseDir)
	r := multitemplate.NewRenderer()

	for _, td := range t.Templates {
		if util.LenTrim(td.LayoutPath) == 0 {
			continue
		}

		layoutPattern := filepath.Join(base, td.LayoutPath)
		layoutFiles, err := filepath.Glob(layoutPattern)

		if err != nil {
			log.Printf("LoadHtmlTemplates: glob layout failed for pattern %q: %v", layoutPattern, err)
			continue
		}
		if len(layoutFiles) == 0 {
			log.Printf("LoadHtmlTemplates: no layout files match pattern %q", layoutPattern)
			continue
		}

		if util.LenTrim(td.PagePath) == 0 {
			if tp := r.AddFromFiles(filepath.Base(layoutFiles[0]), layoutFiles...); tp != nil {
				t._htmlTemplatesCount = len(layoutFiles)
			}
			continue
		}

		pagePattern := filepath.Join(base, td.PagePath)
		pageFiles, err := filepath.Glob(pagePattern)
		if err != nil {
			log.Printf("LoadHtmlTemplates: glob pages failed for pattern %q: %v", pagePattern, err)
			continue
		}
		if len(pageFiles) == 0 {
			log.Printf("LoadHtmlTemplates: no page files match pattern %q", pagePattern)
			continue
		}

		for _, f := range pageFiles {
			layoutCopy := make([]string, len(layoutFiles))
			copy(layoutCopy, layoutFiles)
			files := append(layoutCopy, f)

			if tp := r.AddFromFiles(filepath.Base(f), files...); tp != nil {
				t._htmlTemplatesCount++
			}
		}
	}

	if t._htmlTemplatesCount <= 0 {
		t._htmlrenderer = nil
		return fmt.Errorf("No Html Templates Loaded Into Renderer")
	}

	t._htmlrenderer = r
	return nil
}

// SetHtmlRenderer will set the existing html renderer into gin engine's HTMLRender property
func (t *GinTemplate) SetHtmlRenderer(g *Gin) error {
	if t._htmlrenderer == nil {
		return fmt.Errorf("Html Template Renderer is Required")
	}

	if g == nil {
		return fmt.Errorf("Gin Wrapper is Required")
	}

	if g._ginEngine == nil {
		return fmt.Errorf(("Gin Engine is Required"))
	}

	if t._htmlTemplatesCount <= 0 {
		return fmt.Errorf("No Html Templates Loaded Into Renderer")
	}

	if t._htmlrenderer == nil {
		return fmt.Errorf("Html Renderer Must Not Be Nil")
	}

	g._ginEngine.HTMLRender = t._htmlrenderer
	return nil
}
