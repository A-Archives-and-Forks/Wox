package system

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"wox/common"
	"wox/i18n"
	"wox/plugin"
	"wox/setting/definition"
	"wox/setting/validator"
	"wox/util"
	"wox/util/clipboard"
	"wox/util/selection"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
)

var aiCommandIcon = common.PluginAICommandIcon

type commandSetting struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	Vision  bool   `json:"vision"` // does the command interact with vision
}

type aiStreamPreviewData struct {
	Answer         string `json:"answer"`
	Reasoning      string `json:"reasoning"`
	Status         string `json:"status"`
	StatusLabel    string `json:"statusLabel"`
	ReasoningTitle string `json:"reasoningTitle"`
	AnswerTitle    string `json:"answerTitle"`
}

func (c *commandSetting) AIModel() (model common.Model) {
	err := json.Unmarshal([]byte(c.Model), &model)
	if err != nil {
		return common.Model{}
	}

	return model
}

func init() {
	plugin.AllSystemPlugin = append(plugin.AllSystemPlugin, &Plugin{})
}

type Plugin struct {
	api plugin.API
}

func (c *Plugin) GetMetadata() plugin.Metadata {
	return plugin.Metadata{
		Id:            "c9910664-1c28-47ae-bad6-e7332a02d471",
		Name:          "i18n:plugin_ai_command_plugin_name",
		Author:        "Wox Launcher",
		Website:       "https://github.com/Wox-launcher/Wox",
		Version:       "1.0.0",
		MinWoxVersion: "2.0.0",
		Runtime:       "Go",
		Description:   "i18n:plugin_ai_command_description",
		Icon:          aiCommandIcon.String(),
		Entry:         "",
		TriggerKeywords: []string{
			"ai",
		},
		SupportedOS: []string{
			"Windows",
			"Macos",
			"Linux",
		},
		SettingDefinitions: definition.PluginSettingDefinitions{
			{
				Type: definition.PluginSettingDefinitionTypeTable,
				Value: &definition.PluginSettingValueTable{
					Key:     "commands",
					Title:   "i18n:plugin_ai_command_commands",
					Tooltip: "i18n:plugin_ai_command_commands_tooltip",
					Columns: []definition.PluginSettingValueTableColumn{
						{
							Key:     "name",
							Label:   "i18n:plugin_ai_command_name",
							Type:    definition.PluginSettingValueTableColumnTypeText,
							Width:   100,
							Tooltip: "i18n:plugin_ai_command_name_tooltip",
							Validators: []validator.PluginSettingValidator{
								{
									Type:  validator.PluginSettingValidatorTypeNotEmpty,
									Value: &validator.PluginSettingValidatorNotEmpty{},
								},
							},
						},
						{
							Key:     "command",
							Label:   "i18n:plugin_ai_command_command",
							Type:    definition.PluginSettingValueTableColumnTypeText,
							Width:   80,
							Tooltip: "i18n:plugin_ai_command_command_tooltip",
							Validators: []validator.PluginSettingValidator{
								{
									Type:  validator.PluginSettingValidatorTypeNotEmpty,
									Value: &validator.PluginSettingValidatorNotEmpty{},
								},
							},
						},
						{
							Key:     "model",
							Label:   "i18n:plugin_ai_command_model",
							Type:    definition.PluginSettingValueTableColumnTypeSelectAIModel,
							Width:   100,
							Tooltip: "i18n:plugin_ai_command_model_tooltip",
							Validators: []validator.PluginSettingValidator{
								{
									Type:  validator.PluginSettingValidatorTypeNotEmpty,
									Value: &validator.PluginSettingValidatorNotEmpty{},
								},
							},
						},
						{
							Key:          "prompt",
							Label:        "i18n:plugin_ai_command_prompt",
							Type:         definition.PluginSettingValueTableColumnTypeText,
							TextMaxLines: 10,
							Tooltip:      "i18n:plugin_ai_command_prompt_tooltip",
							Validators: []validator.PluginSettingValidator{
								{
									Type:  validator.PluginSettingValidatorTypeNotEmpty,
									Value: &validator.PluginSettingValidatorNotEmpty{},
								},
							},
						},
						{
							Key:     "vision",
							Label:   "i18n:plugin_ai_command_vision",
							Type:    definition.PluginSettingValueTableColumnTypeCheckbox,
							Width:   60,
							Tooltip: "i18n:plugin_ai_command_vision_tooltip",
						},
					},
				},
			},
		},
		Features: []plugin.MetadataFeature{
			{
				Name: plugin.MetadataFeatureQuerySelection,
			},
			{
				Name: plugin.MetadataFeatureAI,
			},
		},
	}
}

func (c *Plugin) Init(ctx context.Context, initParams plugin.InitParams) {
	c.api = initParams.API
	c.api.OnSettingChanged(ctx, func(callbackCtx context.Context, key string, value string) {
		if key == "commands" {
			c.api.Log(callbackCtx, plugin.LogLevelInfo, fmt.Sprintf("ai command setting changed: %s", value))
			var commands []plugin.MetadataCommand
			gjson.Parse(value).ForEach(func(_, command gjson.Result) bool {
				commands = append(commands, plugin.MetadataCommand{
					Command:     command.Get("command").String(),
					Description: common.I18nString(command.Get("name").String()),
				})

				return true
			})
			c.api.Log(callbackCtx, plugin.LogLevelInfo, fmt.Sprintf("registering query commands: %v", commands))
			c.api.RegisterQueryCommands(callbackCtx, commands)
		}
	})
}

func (c *Plugin) Query(ctx context.Context, query plugin.Query) plugin.QueryResponse {
	if query.Type == plugin.QueryTypeSelection {
		return plugin.NewQueryResponse(c.querySelection(ctx, query))
	}

	if query.Command == "" {
		return plugin.NewQueryResponse(c.listAllCommands(ctx, query))
	}

	return plugin.NewQueryResponse(c.queryCommand(ctx, query))
}

func (c *Plugin) buildAIStreamPreview(ctx context.Context, streamResult common.ChatStreamData, modelLabel string) plugin.WoxPreview {
	statusLabel := i18n.GetI18nManager().TranslateWox(ctx, "plugin_ai_command_answering")
	if streamResult.Status == common.ChatStreamStatusFinished {
		statusLabel = i18n.GetI18nManager().TranslateWox(ctx, "plugin_ai_command_preview_finished")
	}
	if streamResult.Status == common.ChatStreamStatusError {
		statusLabel = i18n.GetI18nManager().TranslateWox(ctx, "plugin_ai_command_preview_error")
	}

	previewData, err := json.Marshal(aiStreamPreviewData{
		Answer:         streamResult.Data,
		Reasoning:      streamResult.Reasoning,
		Status:         string(streamResult.Status),
		StatusLabel:    statusLabel,
		ReasoningTitle: i18n.GetI18nManager().TranslateWox(ctx, "plugin_ai_command_preview_reasoning"),
		AnswerTitle:    i18n.GetI18nManager().TranslateWox(ctx, "plugin_ai_command_preview_answer"),
	})
	if err != nil {
		// Streaming output can still fall back to markdown because the action is
		// already running. The structured type is only needed for clearer visual
		// separation between reasoning and answer, not for correctness.
		c.api.Log(ctx, plugin.LogLevelWarning, fmt.Sprintf("failed to marshal ai stream preview: %s", err.Error()))
		return plugin.WoxPreview{PreviewType: plugin.WoxPreviewTypeMarkdown, PreviewData: streamResult.ToMarkdown()}
	}

	// Keep metadata in preview properties so WoxPreviewScaffold renders it as
	// the same external pill strip used by text, clipboard, and file previews.
	return plugin.WoxPreview{
		PreviewType:       plugin.WoxPreviewTypeAIStream,
		PreviewData:       string(previewData),
		PreviewProperties: map[string]string{"i18n:plugin_ai_command_model": modelLabel},
		ScrollPosition:    plugin.WoxPreviewScrollPositionBottom,
	}
}

func (c *Plugin) buildSelectionPreview(ctx context.Context, command commandSetting, query plugin.Query) plugin.WoxPreview {
	model := command.AIModel()
	modelLabel := fmt.Sprintf("%s - %s", model.ProviderName(), model.Name)
	previewProperties := map[string]string{
		"i18n:plugin_ai_command_model": modelLabel,
	}

	if query.Selection.Type == selection.SelectionTypeText {
		previewProperties["i18n:plugin_ai_command_preview_selected_text"] = fmt.Sprintf(i18n.GetI18nManager().TranslateWox(ctx, "plugin_ai_command_selection_characters_value"), len([]rune(query.Selection.Text)))
		// AI command selection previews do not need a dedicated type: before the
		// model runs, the most useful preview is the selected text itself. Reusing
		// the shared text renderer keeps visual behavior consistent with clipboard
		// and normal selection previews.
		return plugin.WoxPreview{PreviewType: plugin.WoxPreviewTypeText, PreviewData: query.Selection.Text, PreviewProperties: previewProperties}
	}

	if query.Selection.Type == selection.SelectionTypeFile {
		previewProperties["i18n:plugin_ai_command_preview_selected_files"] = fmt.Sprintf(i18n.GetI18nManager().TranslateWox(ctx, "selection_files_count_value"), len(query.Selection.FilePaths))
		items := make([]plugin.WoxPreviewListItem, 0, len(query.Selection.FilePaths))
		for _, filePath := range query.Selection.FilePaths {
			icon := common.NewWoxImageFileIcon(filePath)
			extension := strings.TrimPrefix(filepath.Ext(filePath), ".")
			typeLabel := strings.ToUpper(extension)
			if typeLabel == "" {
				typeLabel = "FILE"
			}

			items = append(items, plugin.WoxPreviewListItem{
				Icon:     &icon,
				Title:    filepath.Base(filePath),
				Subtitle: filepath.Dir(filePath),
				Tails:    []plugin.QueryResultTail{plugin.NewQueryResultTailText(typeLabel)},
			})
		}

		// AI commands now share the generic list preview contract with normal
		// selection results. The old file-only payload could not represent the
		// progress/status rows needed by long-running plugin actions.
		previewJson, err := json.Marshal(plugin.WoxPreviewListData{Items: items})
		if err != nil {
			// If JSON encoding fails, keep the legacy hint rather than blocking
			// the command from running.
			c.api.Log(ctx, plugin.LogLevelWarning, fmt.Sprintf("failed to marshal ai command file selection preview: %s", err.Error()))
			return plugin.WoxPreview{PreviewType: plugin.WoxPreviewTypeMarkdown, PreviewData: "i18n:plugin_ai_command_enter_to_start"}
		}
		return plugin.WoxPreview{PreviewType: plugin.WoxPreviewTypeList, PreviewData: string(previewJson), PreviewProperties: previewProperties}
	}

	return plugin.WoxPreview{PreviewType: plugin.WoxPreviewTypeMarkdown, PreviewData: "i18n:plugin_ai_command_enter_to_start", PreviewProperties: previewProperties}
}

func (c *Plugin) querySelection(ctx context.Context, query plugin.Query) []plugin.QueryResult {
	commands, commandsErr := c.getAllCommands(ctx)
	if commandsErr != nil {
		return []plugin.QueryResult{}
	}

	var results []plugin.QueryResult
	for _, command := range commands {
		if query.Selection.Type == selection.SelectionTypeFile {
			if !command.Vision {
				continue
			}
		}
		if query.Selection.Type == selection.SelectionTypeText {
			if command.Vision {
				continue
			}
		}

		var conversations []common.Conversation
		if query.Selection.Type == selection.SelectionTypeFile {
			var images []common.WoxImage
			for _, imagePath := range query.Selection.FilePaths {
				images = append(images, common.WoxImage{
					ImageType: common.WoxImageTypeAbsolutePath,
					ImageData: imagePath,
				})
			}
			conversations = append(conversations, common.Conversation{
				Role:   common.ConversationRoleUser,
				Text:   command.Prompt,
				Images: images,
			})
		}
		if query.Selection.Type == selection.SelectionTypeText {
			conversations = append(conversations, common.Conversation{
				Role: common.ConversationRoleUser,
				Text: fmt.Sprintf(command.Prompt, query.Selection.Text),
			})
		}

		model := command.AIModel()
		modelLabel := fmt.Sprintf("%s - %s", model.ProviderName(), model.Name)
		result := plugin.QueryResult{
			Id:       uuid.NewString(),
			Title:    command.Name,
			SubTitle: modelLabel,
			Icon:     aiCommandIcon,
			Preview:  c.buildSelectionPreview(ctx, command, query),
			Actions: []plugin.QueryResultAction{
				{
					Name:                   "i18n:plugin_ai_command_run",
					PreventHideAfterAction: true,
					Action: func(ctx context.Context, actionContext plugin.ActionContext) {
						util.Go(ctx, "ai command stream", func() {
							var startAnsweringTime int64

							// Show preparing state
							if updatable := c.api.GetUpdatableResult(ctx, actionContext.ResultId); updatable != nil {
								subTitle := "i18n:plugin_ai_command_answering"
								preview := c.buildAIStreamPreview(ctx, common.ChatStreamData{Status: common.ChatStreamStatusStreaming}, modelLabel)
								updatable.Preview = &preview
								updatable.SubTitle = &subTitle
								startAnsweringTime = util.GetSystemTimestamp()
								if !c.api.UpdateResult(ctx, *updatable) {
									return
								}
							}

							// Start streaming
							err := c.api.AIChatStream(ctx, command.AIModel(), conversations, common.EmptyChatOptions, func(streamResult common.ChatStreamData) {
								updatable := c.api.GetUpdatableResult(ctx, actionContext.ResultId)
								if updatable == nil {
									return
								}

								switch streamResult.Status {
								case common.ChatStreamStatusStreaming:
									subTitle := "i18n:plugin_ai_command_answering"
									preview := c.buildAIStreamPreview(ctx, streamResult, modelLabel)
									updatable.SubTitle = &subTitle
									updatable.Preview = &preview
									c.api.UpdateResult(ctx, *updatable)

								case common.ChatStreamStatusFinished:
									subTitle := fmt.Sprintf(i18n.GetI18nManager().TranslateWox(ctx, "plugin_ai_command_answered_cost"), util.GetSystemTimestamp()-startAnsweringTime)
									preview := c.buildAIStreamPreview(ctx, streamResult, modelLabel)
									actions := []plugin.QueryResultAction{
										{
											Name: "i18n:plugin_ai_command_copy",
											Icon: common.CopyIcon,
											Action: func(ctx context.Context, actionContext plugin.ActionContext) {
												clipboard.WriteText(streamResult.Data)
											},
										},
									}
									pasteToActiveWindowAction, pasteToActiveWindowErr := GetPasteToActiveWindowAction(ctx, c.api, query.Env.ActiveWindowTitle, query.Env.ActiveWindowPid, query.Env.ActiveWindowIcon, func() {
										clipboard.WriteText(streamResult.Data)
									})
									if pasteToActiveWindowErr == nil {
										actions = append(actions, pasteToActiveWindowAction)
									}
									updatable.SubTitle = &subTitle
									updatable.Preview = &preview
									updatable.Actions = &actions
									c.api.UpdateResult(ctx, *updatable)

								case common.ChatStreamStatusError:
									preview := c.buildAIStreamPreview(ctx, streamResult, modelLabel)
									updatable.Preview = &preview
									c.api.UpdateResult(ctx, *updatable)
								}
							})

							if err != nil {
								if updatable := c.api.GetUpdatableResult(ctx, actionContext.ResultId); updatable != nil && updatable.Preview != nil {
									preview := c.buildAIStreamPreview(ctx, common.ChatStreamData{Status: common.ChatStreamStatusError, Data: err.Error()}, modelLabel)
									updatable.Preview = &preview
									c.api.UpdateResult(ctx, *updatable)
								}
							}
						})
					},
				},
			},
		}
		results = append(results, result)
	}
	return results
}

func (c *Plugin) listAllCommands(ctx context.Context, query plugin.Query) []plugin.QueryResult {
	commands, commandsErr := c.getAllCommands(ctx)
	if commandsErr != nil {
		return []plugin.QueryResult{
			{
				Title:    "Failed to get ai commands",
				SubTitle: commandsErr.Error(),
				Icon:     aiCommandIcon,
			},
		}
	}

	if len(commands) == 0 {
		return []plugin.QueryResult{
			{
				Title: "i18n:plugin_ai_command_no_commands",
				Icon:  aiCommandIcon,
			},
		}
	}

	var results []plugin.QueryResult
	for _, command := range commands {
		results = append(results, plugin.QueryResult{
			Title:    command.Command,
			SubTitle: command.Name,
			Icon:     aiCommandIcon,
			Actions: []plugin.QueryResultAction{
				{
					Name:                   "i18n:plugin_ai_command_run",
					PreventHideAfterAction: true,
					Action: func(ctx context.Context, actionContext plugin.ActionContext) {
						c.api.ChangeQuery(ctx, common.PlainQuery{
							QueryType: plugin.QueryTypeInput,
							QueryText: fmt.Sprintf("%s %s ", query.TriggerKeyword, command.Command),
						})
					},
				},
			},
		})
	}
	return results
}

func (c *Plugin) getAllCommands(ctx context.Context) (commands []commandSetting, err error) {
	commandSettings := c.api.GetSetting(ctx, "commands")
	if commandSettings == "" {
		return nil, nil
	}

	err = json.Unmarshal([]byte(commandSettings), &commands)
	return
}

func (c *Plugin) queryCommand(ctx context.Context, query plugin.Query) []plugin.QueryResult {
	if query.Search == "" {
		return []plugin.QueryResult{
			{
				Title: "i18n:plugin_ai_command_type_to_start",
				Icon:  aiCommandIcon,
			},
		}
	}

	commands, commandsErr := c.getAllCommands(ctx)
	if commandsErr != nil {
		return []plugin.QueryResult{
			{
				Title:    "Failed to get ai commands",
				SubTitle: commandsErr.Error(),
				Icon:     aiCommandIcon,
			},
		}
	}
	if len(commands) == 0 {
		return []plugin.QueryResult{
			{
				Title: "i18n:plugin_ai_command_no_commands",
				Icon:  aiCommandIcon,
			},
		}
	}

	aiCommandSetting, commandExist := lo.Find(commands, func(tool commandSetting) bool {
		return tool.Command == query.Command
	})
	if !commandExist {
		return []plugin.QueryResult{
			{
				Title: "i18n:plugin_ai_command_not_found",
				Icon:  aiCommandIcon,
			},
		}
	}

	if aiCommandSetting.Prompt == "" {
		return []plugin.QueryResult{
			{
				Title: "i18n:plugin_ai_command_empty_prompt",
				Icon:  aiCommandIcon,
			},
		}
	}

	var prompts = strings.Split(aiCommandSetting.Prompt, "{wox:new_ai_conversation}")
	var conversations []common.Conversation
	for index, message := range prompts {
		msg := fmt.Sprintf(message, query.Search)
		if index%2 == 0 {
			conversations = append(conversations, common.Conversation{
				Role: common.ConversationRoleUser,
				Text: msg,
			})
		} else {
			conversations = append(conversations, common.Conversation{
				Role: common.ConversationRoleAssistant,
				Text: msg,
			})
		}
	}

	var contextData string
	chatModelLabel := fmt.Sprintf("%s - %s", aiCommandSetting.AIModel().Provider, aiCommandSetting.AIModel().Name)
	result := plugin.QueryResult{
		Id:       uuid.NewString(),
		Title:    fmt.Sprintf(i18n.GetI18nManager().TranslateWox(ctx, "plugin_ai_command_chat_with"), aiCommandSetting.Name),
		SubTitle: chatModelLabel,
		Preview:  plugin.WoxPreview{PreviewType: plugin.WoxPreviewTypeMarkdown, PreviewData: ""},
		Icon:     aiCommandIcon,
		Actions: []plugin.QueryResultAction{
			{
				Name: "i18n:plugin_ai_command_copy",
				Icon: common.CopyIcon,
				Action: func(ctx context.Context, actionContext plugin.ActionContext) {
					// contextData is pure content (Reasoning is separated)
					clipboard.WriteText(contextData)
				},
			},
		},
	}

	// paste to active window
	pasteToActiveWindowAction, pasteToActiveWindowErr := GetPasteToActiveWindowAction(ctx, c.api, query.Env.ActiveWindowTitle, query.Env.ActiveWindowPid, query.Env.ActiveWindowIcon, func() {
		// contextData is pure content (Reasoning is separated)
		clipboard.WriteText(contextData)
	})
	if pasteToActiveWindowErr == nil {
		result.Actions = append(result.Actions, pasteToActiveWindowAction)
	}

	// Start LLM stream immediately when result is displayed
	util.Go(ctx, "ai chat stream", func() {
		err := c.api.AIChatStream(ctx, aiCommandSetting.AIModel(), conversations, common.EmptyChatOptions, func(streamResult common.ChatStreamData) {
			updatable := c.api.GetUpdatableResult(ctx, result.Id)
			if updatable == nil {
				return
			}

			switch streamResult.Status {
			case common.ChatStreamStatusStreaming:
				contextData = streamResult.Data
				preview := c.buildAIStreamPreview(ctx, streamResult, chatModelLabel)
				updatable.Preview = &preview
				c.api.UpdateResult(ctx, *updatable)

			case common.ChatStreamStatusFinished:
				contextData = streamResult.Data
				preview := c.buildAIStreamPreview(ctx, streamResult, chatModelLabel)
				updatable.Preview = &preview
				c.api.UpdateResult(ctx, *updatable)

			case common.ChatStreamStatusError:
				preview := c.buildAIStreamPreview(ctx, streamResult, chatModelLabel)
				updatable.Preview = &preview
				c.api.UpdateResult(ctx, *updatable)
			}
		})

		if err != nil {
			if updatable := c.api.GetUpdatableResult(ctx, result.Id); updatable != nil && updatable.Preview != nil {
				preview := c.buildAIStreamPreview(ctx, common.ChatStreamData{Status: common.ChatStreamStatusError, Data: err.Error()}, chatModelLabel)
				updatable.Preview = &preview
				c.api.UpdateResult(ctx, *updatable)
			}
		}
	})

	return []plugin.QueryResult{result}
}
