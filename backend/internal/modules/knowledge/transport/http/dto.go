package http

import knowledgedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"

type DocumentResponse struct {
	ID            int64  `json:"id"`
	Version       int64  `json:"version"`
	RevisionNo    int64  `json:"revisionNo"`
	Type          string `json:"type"`
	VaultPath     string `json:"vaultPath"`
	ContentHash   string `json:"contentHash"`
	GeneratedHash string `json:"generatedHash"`
	Status        string `json:"status"`
	EventID       *int64 `json:"eventID,omitempty"`
	TopicID       *int64 `json:"topicID,omitempty"`
	ReportID      *int64 `json:"reportID,omitempty"`
}

type ProposalResponse struct {
	ID                  int64  `json:"id"`
	Version             int64  `json:"version"`
	DocumentID          int64  `json:"documentID"`
	BaseRevisionNo      int64  `json:"baseRevisionNo"`
	BaseHash            string `json:"baseHash"`
	ProposedFrontmatter string `json:"proposedFrontmatter"`
	ProposedBody        string `json:"proposedBody"`
	DiffSummary         string `json:"diffSummary"`
	Reason              string `json:"reason"`
	Status              string `json:"status"`
}

type ReconciliationIssueResponse struct {
	Path         string `json:"path"`
	Kind         string `json:"kind"`
	ExpectedHash string `json:"expectedHash"`
	ActualHash   string `json:"actualHash"`
}

type ReconciliationResponse struct {
	Scanned  int                           `json:"scanned"`
	Changed  int                           `json:"changed"`
	Conflict int                           `json:"conflict"`
	Issues   []ReconciliationIssueResponse `json:"issues"`
}

func documentResponse(document knowledgedomain.Document) DocumentResponse {
	return DocumentResponse{
		ID: document.ID, Version: document.Version, RevisionNo: document.RevisionNo, Type: string(document.Type),
		VaultPath: document.VaultPath, ContentHash: document.ContentHash, GeneratedHash: document.GeneratedHash,
		Status: string(document.Status), EventID: document.EventID, TopicID: document.TopicID, ReportID: document.ReportID,
	}
}

func proposalResponse(proposal knowledgedomain.Proposal) ProposalResponse {
	return ProposalResponse{
		ID: proposal.ID, Version: proposal.Version, DocumentID: proposal.DocumentID, BaseRevisionNo: proposal.BaseRevisionNo,
		BaseHash: proposal.BaseHash, ProposedFrontmatter: proposal.ProposedFrontmatter, ProposedBody: proposal.ProposedBody,
		DiffSummary: proposal.DiffSummary, Reason: proposal.Reason, Status: string(proposal.Status),
	}
}

func reconciliationResponse(report knowledgedomain.ReconciliationReport) ReconciliationResponse {
	issues := make([]ReconciliationIssueResponse, 0, len(report.Issues))
	for _, issue := range report.Issues {
		issues = append(issues, ReconciliationIssueResponse{Path: issue.Path, Kind: issue.Kind, ExpectedHash: issue.ExpectedHash, ActualHash: issue.ActualHash})
	}
	return ReconciliationResponse{Scanned: report.Scanned, Changed: report.Changed, Conflict: report.Conflict, Issues: issues}
}
