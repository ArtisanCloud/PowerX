package knowledge_content

import (
	"errors"
	"net/url"
	"strconv"

	svc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
)

func Query(values url.Values, space string) (svc.ContentOverviewQuery, error) {
	out := svc.ContentOverviewQuery{Schema: svc.ContentOverviewQuerySchema, SpaceUUID: space, Mode: values.Get("mode"), RequiredCorpusGeneration: values.Get("required_corpus_generation"), Cursor: values.Get("cursor")}
	for key, v := range values {
		if len(v) != 1 {
			return out, svc.KnowledgeInvalidArgumentError(errors.New("duplicate query parameter"))
		}
		switch key {
		case "mode", "required_corpus_generation", "cursor", "page_size", "preview_codepoints":
		default:
			return out, svc.KnowledgeInvalidArgumentError(errors.New("unknown query parameter"))
		}
	}
	for name, target := range map[string]**int{"page_size": &out.PageSize, "preview_codepoints": &out.PreviewCodepoints} {
		if raw, ok := values[name]; ok {
			value, err := strconv.Atoi(raw[0])
			if err != nil {
				return out, svc.KnowledgeInvalidArgumentError(err)
			}
			*target = &value
		}
	}
	return out, nil
}
