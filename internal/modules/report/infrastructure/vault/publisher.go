package vault

import (
	"context"
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/infrastructure/vault"
	"github.com/StephenQiu30/hotkey-server/internal/modules/report/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/report/domain"
)

type Publisher struct{ writer *vault.Writer }

func NewPublisher(root string) *Publisher { return &Publisher{writer: vault.NewWriter(root)} }

func (publisher *Publisher) Publish(_ context.Context, report domain.Report) error {
	if publisher == nil || publisher.writer == nil {
		return fmt.Errorf("report Vault publisher is unavailable")
	}
	var body strings.Builder
	fmt.Fprintf(&body, "# %s\n\n", report.Title)
	body.WriteString(report.Summary)
	body.WriteString("\n\n")
	for _, item := range report.Items {
		fmt.Fprintf(&body, "## %d. %s\n\n%s\n\n", item.Rank, item.Title, item.Summary)
	}
	_, err := publisher.writer.WriteAutomatic("reports", fmt.Sprintf("%s-%d-v%d", report.Type, report.ID, report.Version), body.String())
	return err
}

var _ application.Publisher = (*Publisher)(nil)
