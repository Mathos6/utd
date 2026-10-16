package cmd

import (
	"reflect"
	"testing"

	"github.com/mathieu/utd/internal/utd"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCmd  string
		wantRest []string
		check    func(*testing.T, *utd.Options)
	}{
		{
			name:     "plain command with flags",
			args:     []string{"ffmpeg", "-i", "in.mp4", "out.avi"},
			wantCmd:  "ffmpeg",
			wantRest: []string{"-i", "in.mp4", "out.avi"},
		},
		{
			name:     "command flag looking like a utd flag is passed through",
			args:     []string{"figlet", "-f", "slant", "hi"},
			wantCmd:  "figlet",
			wantRest: []string{"-f", "slant", "hi"},
		},
		{
			name:    "delete equals form",
			args:    []string{"--delete=now", "sl"},
			wantCmd: "sl",
			check: func(t *testing.T, o *utd.Options) {
				if o.Delete != utd.DeleteNow {
					t.Fatalf("delete = %q", o.Delete)
				}
			},
		},
		{
			name:     "delete space form",
			args:     []string{"--delete", "never", "figlet", "x"},
			wantCmd:  "figlet",
			wantRest: []string{"x"},
			check: func(t *testing.T, o *utd.Options) {
				if o.Delete != utd.DeleteNever {
					t.Fatalf("delete = %q", o.Delete)
				}
			},
		},
		{
			name:    "boolean flags",
			args:    []string{"-v", "--no-deps", "figlet"},
			wantCmd: "figlet",
			check: func(t *testing.T, o *utd.Options) {
				if !o.Verbose || !o.NoDeps {
					t.Fatalf("options = %+v", o)
				}
			},
		},
		{
			name:     "package override",
			args:     []string{"--package=ripgrep", "rg", "--version"},
			wantCmd:  "rg",
			wantRest: []string{"--version"},
			check: func(t *testing.T, o *utd.Options) {
				if o.Package != "ripgrep" {
					t.Fatalf("package = %q", o.Package)
				}
			},
		},
		{
			name:    "binary override",
			args:    []string{"--binary", "batcat", "bat"},
			wantCmd: "bat",
			check: func(t *testing.T, o *utd.Options) {
				if o.Binary != "batcat" {
					t.Fatalf("binary = %q", o.Binary)
				}
			},
		},
		{
			name:     "double dash stops parsing",
			args:     []string{"--no-deps", "--", "figlet", "-x"},
			wantCmd:  "figlet",
			wantRest: []string{"-x"},
		},
		{
			name:    "help",
			args:    []string{"--help"},
			wantCmd: "",
			check: func(t *testing.T, o *utd.Options) {
				if !o.Help {
					t.Fatal("help not set")
				}
			},
		},
		{
			name:    "version",
			args:    []string{"--version"},
			wantCmd: "",
			check: func(t *testing.T, o *utd.Options) {
				if !o.ShowVersion {
					t.Fatal("version not set")
				}
			},
		},
		{
			name:    "no arguments",
			args:    []string{},
			wantCmd: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := utd.NewOptions()
			gotCmd, gotRest, err := parseArgs(tt.args, opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotCmd != tt.wantCmd {
				t.Fatalf("command = %q, want %q", gotCmd, tt.wantCmd)
			}
			if !reflect.DeepEqual(gotRest, tt.wantRest) {
				t.Fatalf("rest = %#v, want %#v", gotRest, tt.wantRest)
			}
			if tt.check != nil {
				tt.check(t, opts)
			}
		})
	}
}

func TestParseArgsErrors(t *testing.T) {
	cases := [][]string{
		{"--unknown-flag", "figlet"},
		{"--delete", "sometimes", "figlet"},
		{"--delete"},
		{"--package"},
	}
	for _, args := range cases {
		if _, _, err := parseArgs(args, utd.NewOptions()); err == nil {
			t.Errorf("parseArgs(%v) expected an error", args)
		}
	}
}

func TestValidateDelete(t *testing.T) {
	opts := utd.NewOptions()
	opts.Delete = "whenever"
	if err := opts.Validate(); err == nil {
		t.Fatal("expected an error for an invalid delete value")
	}
	opts = utd.NewOptions()
	opts.Quiet = true
	opts.Verbose = true
	if err := opts.Validate(); err == nil {
		t.Fatal("expected an error for quiet+verbose")
	}
}
