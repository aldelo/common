package gin

/*
 * Copyright 2020 Aldelo, LP
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
	"path/filepath"
)

func NewTemplate(templateBaseDir string, templateLayoutPath string, templateIncludePath string) *GinTemplate {
	return &GinTemplate{
		TemplateBaseDir: templateBaseDir,
		Templates: []TemplateDefintion{
			{
				LayoutPath: templateLayoutPath,
				IncludePath: templateIncludePath,
			},
		},
	}
}

type GinTemplate struct {
	TemplateBaseDir string
	Templates []TemplateDefintion

	_htmlrenderer multitemplate.Renderer
}

type TemplateDefintion struct {
	LayoutPath string
	IncludePath string
}

func (t *GinTemplate) LoadHtmlTemplates() error {
	if util.LenTrim(t.TemplateBaseDir) == 0 {
		return fmt.Errorf("Html Template Base Dir is Required")
	}

	if len(t.Templates) == 0 {
		return fmt.Errorf("Html Template Definition is Required")
	}

	r := multitemplate.NewRenderer()

	if util.Right(t.TemplateBaseDir, 1) == "/" {
		t.TemplateBaseDir = util.Left(t.TemplateBaseDir, len(t.TemplateBaseDir)-1)
	}

	for _, td := range t.Templates {
		if util.LenTrim(td.LayoutPath) > 0 {
			layout := td.LayoutPath
			include := td.IncludePath

			if util.Left(layout, 1) != "/" {
				layout = "/" + layout
			}

			if util.LenTrim(include) > 0 {
				if util.Left(include, 1) != "/" {
					include = "/" + include
				}
			}

			// get all files matching layout pattern
			layoutFiles, err := filepath.Glob(t.TemplateBaseDir + layout)

			if err != nil {
				continue
			}

			if len(layoutFiles) == 0 {
				continue
			}

			if util.LenTrim(include) == 0 {
				// only layout files to add to renderer
				// template name is not important for 'AddFromFiles' (based on AddFromFiles source)
				r.AddFromFiles(filepath.Base(layoutFiles[0]), layoutFiles...)
			} else {
				// has layout and include files to add to renderer
				includeFiles, err := filepath.Glob(t.TemplateBaseDir + include)

				if err != nil {
					continue
				}

				if len(includeFiles) == 0 {
					continue
				}

				// layout with include files to add to renderer
				// template name is not important for 'AddFromFiles' (based on AddFromFiles source)
				for _, f := range includeFiles {
					layoutCopy := make([]string, len(layoutFiles))
					copy(layoutCopy, layoutFiles)
					files := append(layoutCopy, f)
					r.AddFromFiles(filepath.Base(f), files...)
				}
			}
		}
	}

	t._htmlrenderer = r
	return nil
}

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

	g._ginEngine.HTMLRender = t._htmlrenderer
	return nil
}





