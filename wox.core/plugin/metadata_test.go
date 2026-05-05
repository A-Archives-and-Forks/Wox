package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFeatureParamsForGridLayoutParsesJSONNumbers(t *testing.T) {
	metadata := Metadata{
		Features: []MetadataFeature{
			{
				Name: MetadataFeatureGridLayout,
				Params: map[string]any{
					"Columns":     12.0,
					"ShowTitle":   false,
					"ItemPadding": 12.0,
					"ItemMargin":  6.0,
				},
			},
		},
	}

	params, err := metadata.GetFeatureParamsForGridLayout()

	require.NoError(t, err)
	assert.Equal(t, 12, params.Columns)
	assert.False(t, params.ShowTitle)
	assert.Equal(t, 12, params.ItemPadding)
	assert.Equal(t, 6, params.ItemMargin)
}

func TestGetFeatureParamsForDebounceParsesJSONNumbers(t *testing.T) {
	metadata := Metadata{
		Features: []MetadataFeature{
			{
				Name: MetadataFeatureDebounce,
				Params: map[string]any{
					"IntervalMs": 200.0,
				},
			},
		},
	}

	params, err := metadata.GetFeatureParamsForDebounce()

	require.NoError(t, err)
	assert.Equal(t, 200, params.IntervalMs)
}

func TestValidateGlancesRejectsDuplicateIds(t *testing.T) {
	metadata := Metadata{
		Glances: []MetadataGlance{
			{Id: "time", Name: "Time"},
			{Id: "time", Name: "Duplicate Time"},
		},
	}

	err := metadata.ValidateGlances()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate glance id")
}

func TestValidateGlancesAcceptsPluginLocalIds(t *testing.T) {
	metadata := Metadata{
		Glances: []MetadataGlance{
			{Id: "time", Name: "Time", RefreshIntervalMs: 60000},
			{Id: "battery", Name: "Battery", RefreshIntervalMs: 60000},
		},
	}

	err := metadata.ValidateGlances()

	require.NoError(t, err)
}
