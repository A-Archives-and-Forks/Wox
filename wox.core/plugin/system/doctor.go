package system

import (
	"context"
	"wox/plugin"
)

var doctorIcon = plugin.PluginDoctorIcon

func init() {
	plugin.AllSystemPlugin = append(plugin.AllSystemPlugin, &DoctorPlugin{})
}

type DoctorPlugin struct {
	api plugin.API
}

func (r *DoctorPlugin) GetMetadata() plugin.Metadata {
	return plugin.Metadata{
		Id:              "3e7444df-e8d1-44bc-91d3-12a070efb458",
		Name:            "Wox Doctor",
		Author:          "Wox Launcher",
		Website:         "https://github.com/Wox-launcher/Wox",
		Version:         "1.0.0",
		MinWoxVersion:   "2.0.0",
		Runtime:         "Go",
		Description:     "Check your system and Wox settings",
		Icon:            doctorIcon.String(),
		TriggerKeywords: []string{"doctor"},
		SupportedOS:     []string{"Windows", "Macos", "Linux"},
		Features: []plugin.MetadataFeature{
			{
				Name: plugin.MetadataFeatureIgnoreAutoScore,
			},
		},
	}
}

func (r *DoctorPlugin) Init(ctx context.Context, initParams plugin.InitParams) {
	r.api = initParams.API
}

func (r *DoctorPlugin) Query(ctx context.Context, query plugin.Query) (results []plugin.QueryResult) {
	checkResults := plugin.RunDoctorChecks(ctx)

	for _, check := range checkResults {
		icon := plugin.ErrorIcon
		if check.Passed {
			icon = plugin.CorrectIcon
		}

		results = append(results, plugin.QueryResult{
			Title:    check.Name,
			SubTitle: check.Description,
			Icon:     icon,
			Actions: []plugin.QueryResultAction{
				{
					Name: check.ActionName,
					Action: func(ctx context.Context, actionContext plugin.ActionContext) {
						check.Action(ctx)
					},
				},
			},
		})
	}

	return results
}
