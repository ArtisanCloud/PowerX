package evidence

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSourceTokenPreservesLiteralAndLongestDeclaredUnit(t *testing.T) {
	p := policyFixture()
	p.InputFields[0].UnitTokens = []string{"元", "万元"}
	p.InputFields[1].UnitTokens = []string{"元", "万元"}
	input := "30天，投入20万元，产出40万元，原文15%"
	tokens, err := TokenizeSources(map[string]any{"message": input}, []string{"/message"}, p)
	require.NoError(t, err)
	require.Len(t, tokens, 3)
	require.Equal(t, "20", tokens[0].Source.Literal)
	require.Equal(t, "万元", tokens[0].Unit)
	require.Equal(t, "40", tokens[1].Source.Literal)
	require.Equal(t, "%", tokens[2].Unit)
	for _, token := range tokens {
		require.Contains(t, input, token.Source.Quote)
	}
	out, err := ResolveSelection(SourceSelection{Schema: ExtractionSchema, Data: map[string]SelectedValue{"used": {Scope: "s", TokenRef: "token_0"}}}, tokens, p)
	require.NoError(t, err)
	require.Equal(t, "20", out.Data[0].Source.Literal)
	_, err = ResolveSelection(SourceSelection{Schema: ExtractionSchema, Data: map[string]SelectedValue{"used": {Scope: "s", TokenRef: "invented"}}}, tokens, p)
	require.ErrorContains(t, err, "evidence.source_token_missing")
	_, err = ResolveSelection(SourceSelection{Schema: ExtractionSchema, Data: map[string]SelectedValue{"invented": {Scope: "s", TokenRef: "token_0"}}}, tokens, p)
	require.ErrorContains(t, err, "evidence.source_input_key_unknown: invented")
}

func TestSourceTokenPreservesCommasAndWhitespace(t *testing.T) {
	p := policyFixture()
	p.InputFields[0].UnitTokens = []string{"元"}
	tokens, err := TokenizeSources(map[string]any{"message": "cost 80,000 元; count 860"}, []string{"/message"}, p)
	require.NoError(t, err)
	require.Len(t, tokens, 2)
	require.Equal(t, "80,000", tokens[0].Source.Literal)
	require.Equal(t, "元", tokens[0].Unit)
}
