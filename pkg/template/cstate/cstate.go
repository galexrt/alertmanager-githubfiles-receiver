/*
Copyright 2019 Alexander Trost <galexrt@googlemail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cstate

import (
	"fmt"
	"strings"
	"time"

	"github.com/galexrt/alertmanager-githubfiles-receiver/pkg/template"
	"github.com/gernest/front"
	"github.com/spf13/viper"
)

const (
	timeFormat = "2006-01-02 15:04:05"
)

// Template template cstate incident file
func Template(t *template.Templater, currentContent string) (string, error) {
	data := map[string]interface{}{}

	if t.Alert.Status == "resolved" {
		data["State"] = "Resolved"
	} else {
		data["State"] = "Investigating"
	}
	data["StartsAt"] = t.Alert.StartsAt.Format(timeFormat)
	data["EndsAt"] = t.Alert.EndsAt.Format(timeFormat)
	// If the page already exists, gather data from it
	postMortemBlock := ""
	existingEntries := ""
	if currentContent != "" {
		m := front.NewMatter()
		m.Handle("---", front.YAMLHandler)
		f, body, err := m.Parse(strings.NewReader(currentContent))
		if err != nil {
			return "", err
		}
		if startsAt, ok := f["startsAt"]; ok {
			startsAtString, ok := startsAt.(string)
			if !ok {
				return "", fmt.Errorf("failed to type cast startsAt")
			}
			parsed, err := time.Parse(timeFormat, startsAtString)
			if err != nil {
				return "", err
			}
			data["StartsAt"] = parsed.Format(timeFormat)
		}

		bodySplit := strings.Split(body, "---")
		if len(bodySplit) > 1 {
			postMortemBlock = bodySplit[0]
			existingEntries = bodySplit[1]
		} else {
			existingEntries = bodySplit[0]
		}
	}

	newContent := "---\n"
	cstateFrontmatter, err := t.TemplateWithData(viper.GetString("cstateFrontmatter"), data)
	if err != nil {
		return "", err
	}
	newContent += cstateFrontmatter
	newContent += "---\n"

	if postMortemBlock != "" {
		newContent += postMortemBlock
		newContent += "---\n"
	}

	cStateEntry, err := t.TemplateWithData(viper.GetString("cstateEntry"), data)
	if err != nil {
		return "", err
	}
	newContent += "\n" + cStateEntry + "\n" + existingEntries

	return newContent, nil
}
