package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	knowledgedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	knowledgepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/postgres"
	reportdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type reportArchiveDocumentStore interface {
	EnsureReportDocument(context.Context, int64, string, *int64) (knowledgedomain.Document, error)
}

type reportArchiveProposalCreator interface {
	CreateContext(context.Context, int64, int64, string, string, string, string) (knowledgedomain.Proposal, error)
}

// reportArchivePlanner is the composition-root adapter between the report
// publication transaction and the knowledge proposal workflow. It creates a
// reviewable proposal only; Vault remains untouched until an Editor approves
// and explicitly applies that proposal.
type reportArchivePlanner struct {
	documents reportArchiveDocumentStore
	proposals reportArchiveProposalCreator
}

func newReportArchivePlanner(documents *knowledgepostgres.Repository, proposals *knowledgeapplication.ProposalService) *reportArchivePlanner {
	return buildReportArchivePlanner(documents, proposals)
}

func buildReportArchivePlanner(documents reportArchiveDocumentStore, proposals reportArchiveProposalCreator) *reportArchivePlanner {
	return &reportArchivePlanner{documents: documents, proposals: proposals}
}

func (planner *reportArchivePlanner) Prepare(ctx context.Context, report reportdomain.Report) error {
	if planner == nil || planner.documents == nil || planner.proposals == nil {
		return sharedrepository.ErrUnavailable
	}
	if report.Status != reportdomain.ReportPublished || !report.Frozen || report.ReviewedBy == nil || *report.ReviewedBy <= 0 {
		return sharedrepository.ErrInvalidInput
	}
	if err := report.ValidatePublicationShape(); err != nil {
		return fmt.Errorf("%w: invalid published report: %v", sharedrepository.ErrInvalidInput, err)
	}

	title := reportArchiveTitle(report)
	body := renderReportArchiveBody(report)
	for _, marker := range []string{knowledgedomain.AutomaticRegionBegin, knowledgedomain.AutomaticRegionEnd, knowledgedomain.HumanRegionBegin, knowledgedomain.HumanRegionEnd} {
		if strings.Contains(body, marker) {
			return fmt.Errorf("%w: report contains a reserved Vault marker", sharedrepository.ErrInvalidInput)
		}
	}
	if err := knowledgedomain.ValidateVaultMarkdown(title + "\n" + body); err != nil {
		return fmt.Errorf("%w: invalid report projection: %v", sharedrepository.ErrInvalidInput, err)
	}
	frontmatter, err := json.Marshal(struct {
		Title string `json:"title"`
	}{Title: title})
	if err != nil {
		return fmt.Errorf("encode report projection metadata: %w", err)
	}

	vaultPath := fmt.Sprintf("reports/report-%d.md", report.ID)
	document, err := planner.documents.EnsureReportDocument(ctx, report.ID, vaultPath, report.ReviewedBy)
	if err != nil {
		return err
	}
	if err := document.Validate(); err != nil || document.Type != knowledgedomain.DocumentReport || document.ReportID == nil || *document.ReportID != report.ID || document.VaultPath != vaultPath || len(document.ContentHash) != 64 {
		return fmt.Errorf("%w: invalid report knowledge document", sharedrepository.ErrInvalidInput)
	}
	_, err = planner.proposals.CreateContext(ctx, document.ID, document.RevisionNo, document.ContentHash, string(frontmatter), body, "report_published")
	return err
}

func reportArchiveTitle(report reportdomain.Report) string {
	title := "日报"
	if report.Type == reportdomain.ReportWeekly {
		title = "周报"
	}
	return fmt.Sprintf("%s #%d", title, report.ID)
}

func renderReportArchiveBody(report reportdomain.Report) string {
	var body strings.Builder
	body.WriteString("# 日报知识投影\n\n")
	if report.Type == reportdomain.ReportWeekly {
		body.Reset()
		body.WriteString("# 周报知识投影\n\n")
	}
	fmt.Fprintf(&body, "- 报告 ID: `%d`\n- 报告版本: `%d`\n- 周期开始: `%s`\n- 周期结束: `%s`\n\n", report.ID, report.VersionNo, report.Period.Start.UTC().Format("2006-01-02T15:04:05Z07:00"), report.Period.End.UTC().Format("2006-01-02T15:04:05Z07:00"))
	body.WriteString("## 摘要\n\n")
	if strings.TrimSpace(report.Summary) == "" {
		body.WriteString("暂无摘要。\n")
	} else {
		body.WriteString(report.Summary)
		body.WriteByte('\n')
	}
	if strings.TrimSpace(report.Body) != "" {
		body.WriteString("\n## 正文\n\n")
		body.WriteString(report.Body)
		body.WriteByte('\n')
	}
	body.WriteString("\n## 事件与证据\n\n")
	if len(report.Items) == 0 {
		body.WriteString("本周期暂无事件。\n")
		return body.String()
	}
	for _, item := range report.Items {
		fmt.Fprintf(&body, "### %d. %s\n\n", item.Rank, item.Title)
		if strings.TrimSpace(item.Summary) != "" {
			body.WriteString(item.Summary)
			body.WriteString("\n\n")
		}
		fmt.Fprintf(&body, "- Product Event: `%d`，更新: `%d`，摘要: `%d`\n", item.MicroEventID, item.MicroEventUpdateID, item.MicroEventSummaryID)
		for _, sentence := range item.Sentences {
			fmt.Fprintf(&body, "- %s", sentence.Text)
			if len(sentence.ClaimEvidenceVersionIDs) > 0 {
				body.WriteString("（Evidence IDs:")
				for _, evidenceID := range sentence.ClaimEvidenceVersionIDs {
					fmt.Fprintf(&body, " `%d`", evidenceID)
				}
				body.WriteString("）")
			}
			body.WriteByte('\n')
		}
		body.WriteByte('\n')
	}
	return body.String()
}
