package domain

import "testing"

func TestNormalizeSourcePartiesKeepsExplicitRolesAndCanonicalOrder(t *testing.T) {
	parties, err := NormalizeSourceParties([]SourcePartyAssertion{
		{
			Role: SourcePartyRoleDistributor, Kind: SourcePartyKindAccount,
			IdentityNamespace: "  platform-account  ", ExternalID: " distributor-7 ",
			DisplayName: " Syndication Desk ", HomepageURL: "https://distribution.example.test/accounts/7",
		},
		{
			Role: SourcePartyRolePublisher, Kind: SourcePartyKindOrganization,
			IdentityNamespace: " publisher-registry ", ExternalID: "publisher-42",
			DisplayName: " Example Newsroom ", HomepageURL: "https://publisher.example.test/",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeSourceParties(): %v", err)
	}
	if len(parties) != 2 || parties[0].Role != SourcePartyRolePublisher || parties[1].Role != SourcePartyRoleDistributor {
		t.Fatalf("canonical parties = %#v", parties)
	}
	if parties[0].IdentityNamespace != "publisher-registry" || parties[0].DisplayName != "Example Newsroom" {
		t.Fatalf("normalized publisher = %#v", parties[0])
	}
}
func TestNormalizeSourcePartiesRejectsAmbiguousOrUnsafeAssertions(t *testing.T) {
	valid := SourcePartyAssertion{
		Role: SourcePartyRolePublisher, Kind: SourcePartyKindOrganization,
		IdentityNamespace: "publisher-registry", ExternalID: "publisher-42", DisplayName: "Example Newsroom",
	}
	for name, parties := range map[string][]SourcePartyAssertion{
		"two publishers": {valid, {
			Role: SourcePartyRolePublisher, Kind: SourcePartyKindOrganization,
			IdentityNamespace: "publisher-registry", ExternalID: "publisher-43", DisplayName: "Other Newsroom",
		}},
		"duplicate":        {valid, valid},
		"unknown role":     {{Role: "source_operator", Kind: SourcePartyKindOrganization, IdentityNamespace: "registry", ExternalID: "1", DisplayName: "Operator"}},
		"unsafe homepage":  {{Role: SourcePartyRolePublisher, Kind: SourcePartyKindOrganization, IdentityNamespace: "registry", ExternalID: "1", DisplayName: "Publisher", HomepageURL: "https://user:secret@publisher.example.test/"}},
		"guessed identity": {{Role: SourcePartyRolePublisher, Kind: SourcePartyKindOrganization, IdentityNamespace: "", ExternalID: "", DisplayName: "publisher.example.test"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeSourceParties(parties); err == nil {
				t.Fatal("NormalizeSourceParties() succeeded, want fail-closed validation")
			}
		})
	}
}
