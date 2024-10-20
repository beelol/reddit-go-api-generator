package scraper

import (
	"fmt"
	"log"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
)

type RedditAPIField struct {
	Name        string
	Description string
	Type        string
}

type Output struct {
	Name        string
	Description string
	Type        string
}

type Parameter struct {
	Name        string
	Description string
	Type        string
}

type Input struct {
	Name        string
	Description string
	Type        string
}

type Endpoint struct {
	ID          string
	Method      string
	Path        string
	Description string
	URLParams   []string
	Payload     []Input
	Response    []Output
	QueryParams []Parameter
}

// ScrapeRedditAPI scrapes the Reddit API documentation
func ScrapeRedditAPI(limit int, onEndpointTargetted, onEndpointProcessed func(string)) ([]Endpoint, error) {
	var endpoints []Endpoint
	c := colly.NewCollector()

	// c.Limit(&colly.LimitRule{
	// 	DomainGlob:  "*",
	// 	Parallelism: 1,               // Only one request at a time
	// 	Delay:       2 * time.Second, // 2 seconds delay between requests
	// })

	// c.SetRequestTimeout(10 * time.Second) // 10-second timeout for each request

	c.OnError(func(r *colly.Response, err error) {
		fmt.Printf("Request URL: %v failed with response: %v\nError: %v\n", r.Request.URL, r, err)
	})

	c.OnResponse(func(r *colly.Response) {
		fmt.Printf("Visited: %s\n", r.Request.URL)
	})

	count := 0

	c.OnHTML("div.endpoint", func(e *colly.HTMLElement) {

		// limit <= 0 means get all
		if limit > 0 && count >= limit {
			return // Stop processing if we've reached the limit
		}

		method := e.ChildText("h3 span.method")

		path := extractDynamicPath(e)

		id := method + " " + path

		onEndpointTargetted(id)

		description := e.ChildText("div.md p")
		if description == "" {
			description = "No description available"
		}

		urlParams := extractURLParams(e)

		payload := extractPayload(e)
		newPayload, response := extractPayloadOrResponse(e, method)

		queryParams := extractQueryParams(e)

		finalPayload := payload

		if len(newPayload) > len(payload) {
			finalPayload = newPayload
		}

		endpoint := Endpoint{
			ID:          id,
			Method:      method,
			Path:        path,
			Description: description,
			URLParams:   urlParams,
			Payload:     finalPayload,
			Response:    response,
			QueryParams: queryParams,
		}

		onEndpointProcessed(id)
		// log.Printf("Processed endpoint: %s %s", method, path)
		fmt.Printf("Endpoint Path: %s\n", path)

		endpoints = append(endpoints, endpoint)
		count++
	})

	// localFilePath := "./reddit_api.html"

	// // Check if the local HTML file exists
	// if _, err := os.Stat(localFilePath); err == nil {
	// 	// If the local file exists, visit it
	// 	err := c.Visit("file://" + localFilePath)
	// 	if err != nil {
	// 		log.Fatalf("Error visiting local HTML file: %v", err)
	// 	}
	// } else {
	// 	// If the local file does not exist, visit the remote URL
	err := c.Visit("https://www.reddit.com/dev/api/")
	if err != nil {
		log.Fatalf("Error visiting remote URL: %v", err)
	}
	// }

	// if err != nil {
	// 	log.Fatalf("Error visiting URL: %v", err)
	// 	return nil, err
	// }

	return endpoints, nil
}

// Extract URL parameters from placeholders in the path
func extractURLParams(e *colly.HTMLElement) []string {
	var urlParams []string
	e.ForEach("h3 em.placeholder", func(_ int, em *colly.HTMLElement) {
		urlParams = append(urlParams, em.Text)
	})
	return urlParams
}

// Extract payload parameters for POST/PATCH requests
// func extractPayload(e *colly.HTMLElement) []Input {
// 	var inputs []Input
// 	e.ForEach("table.parameters tr.json-model", func(_ int, tr *colly.HTMLElement) {
// 		payload := tr.ChildText("td pre code")
// 		if payload != "" {
// 			lines := strings.Split(payload, "\n")
// 			for _, line := range lines {
// 				line = strings.TrimSpace(line)
// 				if line == "{" || line == "}" || line == "" {
// 					continue
// 				}

// 				name, inputType, description := parsePayloadLine(line)
// 				input := Input{
// 					Name:        name,
// 					Description: description,
// 					Type:        inputType,
// 				}
// 				inputs = append(inputs, input)
// 			}
// 		}
// 	})
// 	return inputs
// }

// Extract payload parameters when indicated in the HTML structure
func extractPayload(e *colly.HTMLElement) []Input {
	var inputs []Input

	// Check if there's a specific indication that this table is for JSON payload
	e.ForEach("table.parameters tr", func(_ int, tr *colly.HTMLElement) {
		header := tr.ChildText("th")

		if strings.Contains(header, "header") {
			return
		}

		// Look for exact text that indicates payload structure
		if strings.Contains(header, "expects JSON data of this format") {

			codeBlock := tr.DOM.Find("td pre code")

			if codeBlock != nil {
				lines := strings.Split(codeBlock.Text(), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line == "{" || line == "}" || line == "" {
						continue
					}

					name, inputType, description := parsePayloadLine(line)
					if name != "" {
						input := Input{
							Name:        name,
							Description: description,
							Type:        inputType,
						}
						inputs = append(inputs, input)
					}
				}
			}
		}
	})

	return inputs
}

// Parses a line of payload and returns the name, type, and description
func parsePayloadLine(line string) (name, inputType, description string) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return "", "", ""
	}

	name = strings.TrimSpace(strings.Trim(parts[0], `"`))
	description = strings.TrimSpace(parts[1])

	inputType = determineType(description)

	return name, inputType, description
}

func extractPayloadOrResponse(e *colly.HTMLElement, method string) ([]Input, []Output) {
	var inputs []Input
	var outputs []Output
	isPayload := false

	// Determine if the table is likely a payload or a response
	switch method {
	case "POST", "PATCH", "PUT":
		// For these methods, if the scope is related to mod actions or submissions, it's a payload
		// if strings.Contains(oauthScope, "modposts") || strings.Contains(oauthScope, "submit") {
		isPayload = true
		// }
	case "GET":
		// For GET methods, if the scope is "read," it's likely a response
		// if strings.Contains(oauthScope, "read") {
		isPayload = false
		// }
	}

	// Extract information based on the determined type
	e.ForEach("table.parameters tbody tr", func(_ int, tr *colly.HTMLElement) {
		paramName := tr.ChildText("th")
		paramDesc := tr.ChildText("td p")

		// Skip header fields
		if strings.Contains(strings.ToLower(paramName), "header") {
			return
		}

		if isPayload {
			inputType := determineType(paramDesc)
			input := Input{
				Name:        paramName,
				Description: paramDesc,
				Type:        inputType,
			}
			inputs = append(inputs, input)
		} else {
			outputType := determineType(paramDesc)
			output := Output{
				Name:        paramName,
				Description: paramDesc,
				Type:        outputType,
			}
			outputs = append(outputs, output)
		}
	})

	return inputs, outputs
}

// Extract response parameters when they don't indicate a payload structure
// func extractResponse(e *colly.HTMLElement) []RedditAPIField {
// 	var response []Output
// 	e.ForEach("table.parameters tr", func(_ int, tr *colly.HTMLElement) {
// 		header := tr.ChildText("th")
// 		// Ensure we are not parsing a table meant for payload
// 		if strings.Contains(header, "expects JSON data of this format") {
// 			// Skip tables that indicate they are payload
// 			return
// 		}

// 		// Extract response parameters
// 		paramName := tr.ChildText("th")
// 		paramDesc := tr.ChildText("td p")
// 		paramType := determineType(paramDesc)

// 		response = append(response, Output{
// 			Name:        paramName,
// 			Description: paramDesc,
// 			Type:        paramType,
// 		})
// 	})
// 	return response
// }

func isPayload(method string) bool {
	isPayload := true

	// Determine if the table is likely a payload or a response
	switch method {
	case "POST", "PATCH", "PUT":
		// For these methods, if the scope is related to mod actions or submissions, it's a payload
		// if strings.Contains(oauthScope, "modposts") || strings.Contains(oauthScope, "submit") {
		isPayload = true
		// }
	case "GET":
		// For GET methods, if the scope is "read," it's likely a response
		// if strings.Contains(oauthScope, "read") {
		isPayload = false
		// }
	}

	return isPayload
}

// Extract query parameters if present
func extractQueryParams(e *colly.HTMLElement) []Parameter {
	var queryParams []Parameter
	e.ForEach("table.parameters tbody tr", func(_ int, tr *colly.HTMLElement) {
		if tr.ChildText("th") == "after" || tr.ChildText("th") == "before" || tr.ChildText("th") == "count" || tr.ChildText("th") == "limit" {
			paramName := tr.ChildText("th")
			paramDesc := tr.ChildText("td p")
			paramType := determineType(paramDesc)

			queryParams = append(queryParams, Parameter{
				Name:        paramName,
				Description: paramDesc,
				Type:        paramType,
			})
		}
	})
	return queryParams
}

// Determine the type of a parameter based on its description
func determineType(description string) string {
	originalDesc := description
	description = strings.ToLower(description)

	switch {
	case strings.Contains(description, "boolean"):
		return "bool"
	case strings.Contains(description, "integer"):
		return "int"
	case strings.Contains(description, "string"):
		return "string"
	case strings.Contains(description, "valid url"):
		return "string"
	case strings.Contains(description, "a valid email"):
		return "string"
	case strings.Contains(description, "fullname"):
		return "string"
	case strings.Contains(description, "one of"):
		// Extract the content between parentheses as the enum values
		start := strings.Index(originalDesc, "one of (")
		end := strings.Index(originalDesc, ")")
		if start != -1 && end != -1 && end > start {
			options := originalDesc[start+8 : end]

			options = strings.ReplaceAll(options, "`", "")

			// Return formatted enum type with options
			return fmt.Sprintf("enum(%s)", options)
		}
	case strings.Contains(description, "expand"):
		return "bool"
	case strings.Contains(description, "alphanumeric"):
		return "string"
	case strings.Contains(description, "characters"):
		return "string"
	// Retain the original cases that may not have been included before
	case strings.Contains(description, "comma-separated"):
		return "string"
	case strings.Contains(description, "optional") && strings.Contains(description, "boolean"):
		return "bool"
	case strings.Contains(description, "optional") && strings.Contains(description, "integer"):
		return "int"
	}

	return "interface{}"
}

// Extract path from the h3 element, excluding oauth-scope-list and other elements
func extractCleanPath(e *colly.HTMLElement) string {
	h3 := e.DOM.Find("h3")

	// Remove any oauth-scope-list and api-badge elements from the h3
	h3.Find("span.oauth-scope-list").Remove()
	h3.Find("a").Remove()

	cleanPath := strings.TrimSpace(h3.Text())
	cleanPath = strings.TrimSpace(strings.Replace(cleanPath, e.ChildText("h3 span.method"), "", 1))

	return cleanPath
}

// Extract dynamic fields (e.g., {where}, [r/subreddit]) from the path
func extractDynamicPath(e *colly.HTMLElement) string {
	h3 := e.DOM.Find("h3")

	// Remove oauth-scope-list and other non-path elements
	h3.Find("span.oauth-scope-list").Remove()
	h3.Find("a").Remove()

	cleanPath := strings.TrimSpace(h3.Text())
	cleanPath = strings.TrimSpace(strings.Replace(cleanPath, e.ChildText("h3 span.method"), "", 1))

	// Replace <em class="placeholder">...</em> elements with dynamic field format
	h3.Find("em.placeholder").Each(func(_ int, el *goquery.Selection) {
		placeholder := el.Text()

		// Replace [r/subreddit] pattern with "r/{subreddit}"
		if placeholder == "subreddit" && strings.Contains(cleanPath, "[r/"+placeholder+"]") {
			cleanPath = strings.ReplaceAll(cleanPath, "[r/"+placeholder+"]", "r/{subreddit}")
		} else {
			// Replace other placeholders like "where" with "{where}"
			cleanPath = strings.ReplaceAll(cleanPath, placeholder, "{"+placeholder+"}")
		}
	})

	// Remove any remaining brackets in the path
	cleanPath = strings.ReplaceAll(cleanPath, "[", "")
	cleanPath = strings.ReplaceAll(cleanPath, "]", "")

	return cleanPath
}
