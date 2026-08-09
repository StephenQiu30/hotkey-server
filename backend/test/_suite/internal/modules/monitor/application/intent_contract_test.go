package application_test

import (
	"reflect"
	"strings"
	"testing"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
)

func TestIntentApplicationContractsDoNotExposeDomainEntities(t *testing.T) {
	t.Parallel()

	contracts := []any{
		monitorapplication.IntentClauseDTO{},
		monitorapplication.IntentEntityDTO{},
		monitorapplication.IntentExampleDTO{},
		monitorapplication.ExpansionCandidateDTO{},
		monitorapplication.IntentDraftDTO{},
		monitorapplication.IntentRunDTO{},
		monitorapplication.ExpansionRunDTO{},
		monitorapplication.PreviewRunDTO{},
		monitorapplication.ReplaceIntentDraftCommand{},
		monitorapplication.ReplaceIntentDraftResult{},
		monitorapplication.ReadIntentDraftQuery{},
		monitorapplication.ReadIntentDraftResult{},
		monitorapplication.ReviewExpansionCandidateCommand{},
		monitorapplication.ReviewExpansionCandidateResult{},
		monitorapplication.SubmitExpansionRunCommand{},
		monitorapplication.SubmitExpansionRunResult{},
		monitorapplication.ReadExpansionRunQuery{},
		monitorapplication.ReadExpansionRunResult{},
		monitorapplication.SubmitPreviewRunCommand{},
		monitorapplication.SubmitPreviewRunResult{},
		monitorapplication.ReadPreviewRunQuery{},
		monitorapplication.ReadPreviewRunResult{},
		monitorapplication.IntentRunReferenceDTO{},
		monitorapplication.StartIntentRunCommand{},
		monitorapplication.StartIntentRunResult{},
		monitorapplication.FailIntentRunCommand{},
		monitorapplication.FailIntentRunResult{},
		monitorapplication.CompleteExpansionRunCommand{},
		monitorapplication.CompleteExpansionRunResult{},
		monitorapplication.CompletePreviewRunCommand{},
		monitorapplication.CompletePreviewRunResult{},
		monitorapplication.IntentRunTransitionDTO{},
		monitorapplication.IntentRunTransitionReceiptDTO{},
		monitorapplication.CompleteExpansionRunMutationDTO{},
		monitorapplication.CompleteExpansionRunReceiptDTO{},
		monitorapplication.CompletePreviewRunMutationDTO{},
		monitorapplication.CompletePreviewRunReceiptDTO{},
	}
	for _, contract := range contracts {
		assertNoMonitorDomainType(t, reflect.TypeOf(contract), map[reflect.Type]bool{})
	}
	ports := []reflect.Type{
		reflect.TypeOf((*monitorapplication.IntentDraftRepository)(nil)).Elem(),
		reflect.TypeOf((*monitorapplication.IntentRunRepository)(nil)).Elem(),
	}
	for _, port := range ports {
		assertNoMonitorDomainType(t, port, map[reflect.Type]bool{})
	}
}

func assertNoMonitorDomainType(t *testing.T, value reflect.Type, visited map[reflect.Type]bool) {
	t.Helper()
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Slice || value.Kind() == reflect.Array || value.Kind() == reflect.Map {
		value = value.Elem()
	}
	if strings.Contains(value.PkgPath(), "/modules/monitor/domain") {
		t.Fatalf("application contract exposes Monitor Domain type %s", value)
	}
	if visited[value] {
		return
	}
	visited[value] = true
	switch value.Kind() {
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			assertNoMonitorDomainType(t, value.Field(index).Type, visited)
		}
	case reflect.Interface:
		for index := 0; index < value.NumMethod(); index++ {
			assertNoMonitorDomainType(t, value.Method(index).Type, visited)
		}
	case reflect.Func:
		for index := 0; index < value.NumIn(); index++ {
			assertNoMonitorDomainType(t, value.In(index), visited)
		}
		for index := 0; index < value.NumOut(); index++ {
			assertNoMonitorDomainType(t, value.Out(index), visited)
		}
	}
}
