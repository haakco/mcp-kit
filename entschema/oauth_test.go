package entschema_test

import (
	"os/exec"
	"reflect"
	"testing"

	"entgo.io/ent"
	_ "entgo.io/ent/dialect/sql/schema"

	"github.com/haakco/mcp-kit/entschema"
)

func TestOAuthClientMixinFieldsAndIndexes(t *testing.T) {
	mixin := entschema.OAuthClient{}
	assertFieldNames(t, mixin, []string{
		"client_id",
		"client_secret_hash",
		"redirect_uris",
		"grant_types",
		"response_types",
		"scopes",
		"audience",
		"is_public",
		"name",
		"workspace_id",
	})
	assertIndex(t, mixin, []string{"client_id"}, false)
	assertIndex(t, mixin, []string{"workspace_id"}, false)
}

func TestOAuthSigningKeyMixinFieldsAndIndexes(t *testing.T) {
	mixin := entschema.OAuthSigningKey{}
	assertFieldNames(t, mixin, []string{
		"kid",
		"algorithm",
		"private_key_pem",
		"public_key_pem",
		"is_active",
		"expires_at",
		"retired_at",
	})
	assertIndex(t, mixin, []string{"kid"}, false)
	assertIndex(t, mixin, []string{"is_active"}, false)
}

func TestOAuthSessionMixinFieldsAndIndexes(t *testing.T) {
	mixin := entschema.OAuthAuthorizationCode{}
	assertFieldNames(t, mixin, []string{
		"session_type",
		"signature",
		"client_id",
		"user_id",
		"request_id",
		"scopes",
		"session_data",
		"active",
		"expires_at",
	})
	assertIndex(t, mixin, []string{"session_type", "signature"}, true)
	assertIndex(t, mixin, []string{"client_id"}, false)
	assertIndex(t, mixin, []string{"user_id"}, false)
	assertIndex(t, mixin, []string{"request_id"}, false)
	assertIndex(t, mixin, []string{"session_type", "active"}, false)
}

func TestSplitOAuthSessionMixinsShareSessionShape(t *testing.T) {
	want := fieldNames(t, entschema.OAuthAuthorizationCode{})
	for name, mixin := range map[string]interface{ Fields() []ent.Field }{
		"access token":  entschema.OAuthAccessToken{},
		"refresh token": entschema.OAuthRefreshToken{},
	} {
		t.Run(name, func(t *testing.T) {
			if got := fieldNames(t, mixin); !reflect.DeepEqual(got, want) {
				t.Fatalf("fields = %#v, want %#v", got, want)
			}
		})
	}
}

func TestPersonalAccessTokenMixinFieldsAndIndexes(t *testing.T) {
	mixin := entschema.PersonalAccessToken{}
	assertFieldNames(t, mixin, []string{
		"user_id",
		"name",
		"token_hash",
		"token_prefix",
		"last_used_at",
		"expires_at",
	})
	assertIndex(t, mixin, []string{"token_prefix"}, false)
	assertIndex(t, mixin, []string{"user_id"}, false)
}

func TestOAuthClientFieldDescriptors(t *testing.T) {
	fields := fieldsByName(entschema.OAuthClient{})

	for _, name := range []string{"redirect_uris", "grant_types", "response_types"} {
		field := fields[name]
		if field.Default == nil {
			t.Fatalf("%s Default is nil, want empty JSON default", name)
		}
	}
	if !fields["client_secret_hash"].Sensitive {
		t.Fatal("client_secret_hash Sensitive = false, want true")
	}
	if !fields["client_id"].Unique {
		t.Fatal("client_id Unique = false, want true")
	}
}

func TestOAuthSessionFieldDescriptors(t *testing.T) {
	fields := fieldsByName(entschema.OAuthAuthorizationCode{})

	if got := fields["user_id"].Type; got != "int" {
		t.Fatalf("user_id type = %q, want int", got)
	}
	if fields["session_data"].Default == nil {
		t.Fatal("session_data Default is nil, want empty JSON default")
	}
}

func TestPersonalAccessTokenFieldDescriptors(t *testing.T) {
	fields := fieldsByName(entschema.PersonalAccessToken{})

	if got := fields["user_id"].Type; got != "int" {
		t.Fatalf("user_id type = %q, want int", got)
	}
	if !fields["token_hash"].Unique {
		t.Fatal("token_hash Unique = false, want true")
	}
}

func TestEntFixtureCompiles(t *testing.T) {
	command := exec.Command("go", "test", "./...")
	command.Dir = "_test"
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture go test failed: %v\n%s", err, string(output))
	}
}

func assertFieldNames(t *testing.T, mixin interface{ Fields() []ent.Field }, want []string) {
	t.Helper()

	got := fieldNames(t, mixin)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fields = %#v, want %#v", got, want)
	}
}

func fieldNames(t *testing.T, mixin interface{ Fields() []ent.Field }) []string {
	t.Helper()

	fields := mixin.Fields()
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Descriptor().Name)
	}
	return names
}

func fieldsByName(mixin interface{ Fields() []ent.Field }) map[string]fieldDescriptor {
	fields := map[string]fieldDescriptor{}
	for _, field := range mixin.Fields() {
		descriptor := field.Descriptor()
		fields[descriptor.Name] = fieldDescriptor{
			Type:      descriptor.Info.Type.String(),
			Default:   descriptor.Default,
			Optional:  descriptor.Optional,
			Nillable:  descriptor.Nillable,
			Sensitive: descriptor.Sensitive,
			Unique:    descriptor.Unique,
		}
	}
	return fields
}

type fieldDescriptor struct {
	Type      string
	Default   any
	Optional  bool
	Nillable  bool
	Sensitive bool
	Unique    bool
}

func assertIndex(t *testing.T, mixin interface{ Indexes() []ent.Index }, fields []string, unique bool) {
	t.Helper()

	for _, index := range mixin.Indexes() {
		descriptor := index.Descriptor()
		if reflect.DeepEqual(descriptor.Fields, fields) && descriptor.Unique == unique {
			return
		}
	}
	t.Fatalf("missing index fields=%v unique=%v", fields, unique)
}
