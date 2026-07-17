package bootstrap

import "testing"

func TestParseRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input      string
		want       Role
		wantAPI    bool
		wantWorker bool
		wantErr    bool
	}{
		{input: "all", want: RoleAll, wantAPI: true, wantWorker: true},
		{input: "api", want: RoleAPI, wantAPI: true, wantWorker: false},
		{input: "worker", want: RoleWorker, wantAPI: false, wantWorker: true},
		{input: "", wantErr: true},
		{input: "scheduler", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got, err := ParseRole(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParseRole() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRole() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseRole() = %q, want %q", got, tt.want)
			}
			if got.StartsAPI() != tt.wantAPI {
				t.Errorf("StartsAPI() = %v, want %v", got.StartsAPI(), tt.wantAPI)
			}
			if got.StartsWorker() != tt.wantWorker {
				t.Errorf("StartsWorker() = %v, want %v", got.StartsWorker(), tt.wantWorker)
			}
		})
	}
}
