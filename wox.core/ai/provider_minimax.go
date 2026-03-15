package ai

import (
	"context"
	"wox/common"
	"wox/setting"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

var miniMaxModels = []string{
	"MiniMax-M2.5",
	"MiniMax-M2.5-highspeed",
}

func init() {
	providerFactories["minimax"] = NewMiniMaxProvider
}

type MiniMaxProvider struct {
	*OpenAIBaseProvider
}

func (p *MiniMaxProvider) GetIcon() common.WoxImage {
	return common.WoxImage{}
}

func (p *MiniMaxProvider) Models(ctx context.Context) ([]common.Model, error) {
	models := make([]common.Model, 0, len(miniMaxModels))
	for _, modelName := range miniMaxModels {
		models = append(models, common.Model{
			Name:          modelName,
			Provider:      common.ProviderName(p.connectContext.Name),
			ProviderAlias: p.connectContext.Alias,
		})
	}

	return models, nil
}

func (p *MiniMaxProvider) Ping(ctx context.Context) error {
	client := p.getClient(ctx)
	_, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: miniMaxModels[0],
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("ping"),
		},
		MaxCompletionTokens: openai.Int(1),
	})

	return err
}

func NewMiniMaxProvider(ctx context.Context, connectContext setting.AIProvider) Provider {
	if connectContext.Host == "" {
		connectContext.Host = "https://api.minimaxi.com/v1"
	}

	return &MiniMaxProvider{
		OpenAIBaseProvider: NewOpenAIBaseProviderWithOptions(connectContext, OpenAIBaseProviderOptions{
			ChatRequestOptions: func(ctx context.Context, model common.Model, conversations []common.Conversation, options common.ChatOptions) []option.RequestOption {
				return []option.RequestOption{
					option.WithJSONSet("reasoning_split", true),
				}
			},
		}),
	}
}
