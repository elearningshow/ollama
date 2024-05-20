package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ollama/ollama/envconfig"
)

func TestGetBlobsPath(t *testing.T) {
	// GetBlobsPath expects an actual directory to exist
	dir, err := os.MkdirTemp("", "ollama-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	tests := []struct {
		name     string
		digest   string
		expected string
		err      error
	}{
		{
			"empty digest",
			"",
			filepath.Join(dir, "blobs"),
			nil,
		},
		{
			"valid with colon",
			"sha256:456402914e838a953e0cf80caa6adbe75383d9e63584a964f504a7bbb8f7aad9",
			filepath.Join(dir, "blobs", "sha256-456402914e838a953e0cf80caa6adbe75383d9e63584a964f504a7bbb8f7aad9"),
			nil,
		},
		{
			"valid with dash",
			"sha256-456402914e838a953e0cf80caa6adbe75383d9e63584a964f504a7bbb8f7aad9",
			filepath.Join(dir, "blobs", "sha256-456402914e838a953e0cf80caa6adbe75383d9e63584a964f504a7bbb8f7aad9"),
			nil,
		},
		{
			"digest too short",
			"sha256-45640291",
			"",
			ErrInvalidDigestFormat,
		},
		{
			"digest too long",
			"sha256-456402914e838a953e0cf80caa6adbe75383d9e63584a964f504a7bbb8f7aad9aaaaaaaaaa",
			"",
			ErrInvalidDigestFormat,
		},
		{
			"digest invalid chars",
			"../sha256-456402914e838a953e0cf80caa6adbe75383d9e63584a964f504a7bbb8f7a",
			"",
			ErrInvalidDigestFormat,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OLLAMA_MODELS", dir)
			envconfig.LoadConfig()

			got, err := GetBlobsPath(tc.digest)

			require.ErrorIs(t, tc.err, err, tc.name)
			assert.Equal(t, tc.expected, got, tc.name)
		})
	}
}

func TestParseModelPath(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want ModelPath
	}{
		{
			"full path https",
			"https://example.com/ns/repo:tag",
			ModelPath{
				ProtocolScheme: "https",
				Registry:       "example.com",
				Namespace:      "ns",
				Repository:     "repo",
				Tag:            "tag",
			},
		},
		{
			"full path http",
			"http://example.com/ns/repo:tag",
			ModelPath{
				ProtocolScheme: "http",
				Registry:       "example.com",
				Namespace:      "ns",
				Repository:     "repo",
				Tag:            "tag",
			},
		},
		{
			"no protocol",
			"example.com/ns/repo:tag",
			ModelPath{
				ProtocolScheme: "https",
				Registry:       "example.com",
				Namespace:      "ns",
				Repository:     "repo",
				Tag:            "tag",
			},
		},
		{
			"no registry",
			"ns/repo:tag",
			ModelPath{
				ProtocolScheme: "https",
				Registry:       DefaultRegistry,
				Namespace:      "ns",
				Repository:     "repo",
				Tag:            "tag",
			},
		},
		{
			"no namespace",
			"repo:tag",
			ModelPath{
				ProtocolScheme: "https",
				Registry:       DefaultRegistry,
				Namespace:      DefaultNamespace,
				Repository:     "repo",
				Tag:            "tag",
			},
		},
		{
			"no tag",
			"repo",
			ModelPath{
				ProtocolScheme: "https",
				Registry:       DefaultRegistry,
				Namespace:      DefaultNamespace,
				Repository:     "repo",
				Tag:            DefaultTag,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseModelPath(tc.arg)

			if got != tc.want {
				t.Errorf("got: %q want: %q", got, tc.want)
			}
		})
	}
}

func TestMigrateRegistryDomain(t *testing.T) {
	manifests := []string{
		filepath.Join("registry.ollama.ai", "library", "mistral", "latest"),
		filepath.Join("registry.ollama.ai", "library", "llama3", "7b"),
		filepath.Join("ollama.com", "library", "llama3", "7b"),
		filepath.Join("localhost:5000", "library", "llama3", "13b"),
	}

	models := t.TempDir()
	t.Setenv("OLLAMA_MODELS", models)
	envconfig.LoadConfig()

	for _, manifest := range manifests {
		p := filepath.Join(models, "manifests", manifest)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}

		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}

		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if err := migrateRegistryDomain(); err != nil {
		t.Fatal(err)
	}

	var actual []string
	if err := filepath.Walk(models, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !fi.IsDir() {
			rel, err := filepath.Rel(models, p)
			if err != nil {
				return err
			}

			actual = append(actual, rel)
		}

		return nil
	}); err != nil {
		t.Fatal(err)
	}

	expected := []string{
		filepath.Join("manifests", "localhost:5000", "library", "llama3", "13b"),
		filepath.Join("manifests", DefaultRegistry, "library", "llama3", "7b"),
		filepath.Join("manifests", DefaultRegistry, "library", "mistral", "latest"),
		filepath.Join("manifests", "registry.ollama.ai", "library", "llama3", "7b"),
	}

	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Errorf("mismatch (-got +want)\n%s", diff)
	}
}
