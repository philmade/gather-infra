package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// -----------------------------------------------------------------------------
// Request / Response types
// -----------------------------------------------------------------------------

type SkillItem struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Source           string   `json:"source,omitempty"`
	Category         string   `json:"category,omitempty"`
	Installs         float64  `json:"installs"`
	ReviewCount      float64  `json:"review_count"`
	AvgScore         *float64 `json:"avg_score"`
	AvgSecurityScore *float64 `json:"avg_security_score"`
	RankScore        *float64 `json:"rank_score"`
	Created          string   `json:"created"`
}

type ListSkillsInput struct {
	Limit       int    `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Max results to return"`
	Offset      int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
	Q           string `query:"q" doc:"Search query (matches name, description)"`
	Category    string `query:"category" doc:"Filter by category"`
	Sort        string `query:"sort" default:"rank" doc:"Sort by: rank, installs, reviews, security, newest"`
	MinSecurity string `query:"min_security" doc:"Minimum avg security score"`
}

type ListSkillsOutput struct {
	Body struct {
		Skills []SkillItem `json:"skills"`
		Total  int         `json:"total"`
		Limit  int         `json:"limit"`
		Offset int         `json:"offset"`
	}
}

type GetSkillInput struct {
	ID string `path:"id" doc:"Skill ID"`
}

type SkillReviewSummary struct {
	ID              string   `json:"id"`
	Task            string   `json:"task,omitempty"`
	Status          string   `json:"status"`
	Score           *float64 `json:"score"`
	WhatWorked      string   `json:"what_worked,omitempty"`
	WhatFailed      string   `json:"what_failed,omitempty"`
	SkillFeedback   string   `json:"skill_feedback,omitempty"`
	AgentModel      string   `json:"agent_model,omitempty"`
	ExecutionTimeMs *float64 `json:"execution_time_ms"`
	Created         string   `json:"created"`
}

type GetSkillOutput struct {
	Body struct {
		SkillItem
		Reviews []SkillReviewSummary `json:"reviews"`
	}
}

type CreateSkillInput struct {
	Authorization string `header:"Authorization" doc:"Bearer JWT token" required:"true"`
	Body          struct {
		ID          string `json:"id" doc:"Unique skill identifier (e.g. 'anthropics/pdf')" minLength:"1"`
		Name        string `json:"name" doc:"Display name" minLength:"1"`
		Description string `json:"description,omitempty" doc:"Short description" maxLength:"2000"`
		Source      string `json:"source,omitempty" doc:"Source: 'skills.sh' or 'github'"`
		Category    string `json:"category,omitempty" doc:"Category (frontend, backend, devtools, security, ai-agents, mobile, content, design, data, general)"`
	}
}

type CreateSkillOutput struct {
	Status int `header:"Status"`
	Body   SkillItem
}

// -----------------------------------------------------------------------------
// Sort mapping
// -----------------------------------------------------------------------------

var sortMap = map[string]string{
	"rank":     "-rank_score,-review_count",
	"installs": "-installs",
	"reviews":  "-review_count",
	"security": "-avg_security_score",
	"newest":   "",
}

var validSources = map[string]bool{"skills.sh": true, "github": true}

var validCategories = map[string]bool{
	"frontend": true, "backend": true, "devtools": true, "security": true,
	"ai-agents": true, "mobile": true, "content": true, "design": true,
	"data": true, "general": true,
}

// -----------------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------------

func RegisterSkillRoutes(api huma.API, app *pocketbase.PocketBase, jwtKey []byte) {
	huma.Register(api, huma.Operation{
		OperationID: "list-skills",
		Method:      "GET",
		Path:        "/api/skills",
		Summary:     "List skills",
		Description: "List skills sorted by rank, with optional search, category filter, and sorting.",
		Tags:        []string{"Skills"},
	}, func(ctx context.Context, input *ListSkillsInput) (*ListSkillsOutput, error) {
		var filters []string
		params := map[string]any{}

		if input.Q != "" {
			filters = append(filters, "(name ~ {:q} || description ~ {:q})")
			params["q"] = input.Q
		}
		if input.Category != "" {
			filters = append(filters, "category = {:cat}")
			params["cat"] = input.Category
		}
		if input.MinSecurity != "" {
			filters = append(filters, "avg_security_score >= {:minsec}")
			params["minsec"] = input.MinSecurity
		}

		filter := "id != ''"
		if len(filters) > 0 {
			filter += " && " + strings.Join(filters, " && ")
		}

		sort := sortMap[input.Sort]
		if sort == "" {
			sort = sortMap["rank"]
		}

		records, err := app.FindRecordsByFilter("skills", filter, sort, input.Limit, input.Offset, params)
		if err != nil {
			records = nil
		}

		skills := make([]SkillItem, 0, len(records))
		for _, r := range records {
			skills = append(skills, recordToSkillItem(r))
		}

		// Get total count (same filter, no limit)
		total := len(skills)
		if allRecords, err := app.FindRecordsByFilter("skills", filter, "", 0, 0, params); err == nil {
			total = len(allRecords)
		}

		out := &ListSkillsOutput{}
		out.Body.Skills = skills
		out.Body.Total = total
		out.Body.Limit = input.Limit
		out.Body.Offset = input.Offset
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-skill",
		Method:      "GET",
		Path:        "/api/skills/{id}",
		Summary:     "Get skill details",
		Description: "Returns skill details with recent reviews.",
		Tags:        []string{"Skills"},
	}, func(ctx context.Context, input *GetSkillInput) (*GetSkillOutput, error) {
		skill, err := app.FindFirstRecordByData("skills", "name", input.ID)
		if err != nil {
			// Try by PocketBase ID
			skill, err = app.FindRecordById("skills", input.ID)
		}
		if err != nil {
			return nil, huma.Error404NotFound("Skill not found")
		}

		// Get recent reviews
		reviews, _ := app.FindRecordsByFilter("reviews",
			"skill = {:sid}", "", 20, 0,
			map[string]any{"sid": skill.Id})

		reviewItems := make([]SkillReviewSummary, 0, len(reviews))
		for _, r := range reviews {
			item := SkillReviewSummary{
				ID:            r.Id,
				Task:          r.GetString("task"),
				Status:        r.GetString("status"),
				WhatWorked:    r.GetString("what_worked"),
				WhatFailed:    r.GetString("what_failed"),
				SkillFeedback: r.GetString("skill_feedback"),
				AgentModel:    r.GetString("agent_model"),
				Created:       r.GetString("created"),
			}
			if v := r.GetFloat("score"); v > 0 {
				item.Score = &v
			}
			if v := r.GetFloat("execution_time_ms"); v > 0 {
				item.ExecutionTimeMs = &v
			}
			reviewItems = append(reviewItems, item)
		}

		out := &GetSkillOutput{}
		out.Body.SkillItem = recordToSkillItem(skill)
		out.Body.Reviews = reviewItems
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-skill",
		Method:        "POST",
		Path:          "/api/skills",
		Summary:       "Add a skill",
		Description:   "Register a new skill in the marketplace.",
		Tags:          []string{"Skills"},
		DefaultStatus: 201,
	}, func(ctx context.Context, input *CreateSkillInput) (*CreateSkillOutput, error) {
		if _, err := RequireJWT(input.Authorization, jwtKey); err != nil {
			return nil, err
		}

		// Check if skill with this name already exists
		existing, _ := app.FindFirstRecordByData("skills", "name", input.Body.ID)
		if existing != nil {
			return nil, huma.Error409Conflict("Skill already exists")
		}

		source := input.Body.Source
		if !validSources[source] {
			source = "github"
		}
		category := input.Body.Category
		if !validCategories[category] {
			category = ""
		}

		collection, err := app.FindCollectionByNameOrId("skills")
		if err != nil {
			return nil, huma.Error500InternalServerError("skills collection not found")
		}

		record := core.NewRecord(collection)
		record.Set("name", input.Body.ID)
		record.Set("description", input.Body.Description)
		record.Set("source", source)
		record.Set("category", category)

		if err := app.Save(record); err != nil {
			return nil, huma.Error500InternalServerError("Failed to create skill")
		}

		out := &CreateSkillOutput{}
		out.Status = 201
		out.Body = recordToSkillItem(record)
		return out, nil
	})
}

func recordToSkillItem(r *core.Record) SkillItem {
	item := SkillItem{
		ID:          r.Id,
		Name:        r.GetString("name"),
		Description: r.GetString("description"),
		Source:      r.GetString("source"),
		Category:    r.GetString("category"),
		Installs:    r.GetFloat("installs"),
		ReviewCount: r.GetFloat("review_count"),
		Created:     fmt.Sprintf("%v", r.GetDateTime("created")),
	}
	if v := r.GetFloat("avg_score"); v > 0 {
		item.AvgScore = &v
	}
	if v := r.GetFloat("avg_security_score"); v > 0 {
		item.AvgSecurityScore = &v
	}
	if v := r.GetFloat("rank_score"); v > 0 {
		item.RankScore = &v
	}
	return item
}
