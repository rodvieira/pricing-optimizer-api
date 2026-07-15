package usecase

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

func TestRenderStripe(t *testing.T) {
	t.Parallel()

	v := fixtureExportVariation()
	result := renderStripe(v)

	assert.Equal(t, domain.ExportFormatStripe, result.Format)
	assert.Equal(t, "stripe-pricing-table.json", result.Filename)
	assert.Equal(t, "application/json", result.ContentType)

	var table stripePricingTable
	require.NoError(t, json.Unmarshal([]byte(result.Content), &table))
	require.Len(t, table.Prices, 1)

	price := table.Prices[0]
	assert.Equal(t, "Pro", price.Product.Name)
	assert.Equal(t, "For growing teams", price.Product.Description)
	assert.Equal(t, []string{"Feature A", "Feature B"}, price.Product.MarketingFeatures)
	require.NotNil(t, price.UnitAmount)
	assert.Equal(t, 2900, *price.UnitAmount)
	assert.Equal(t, "usd", price.Currency)
	require.NotNil(t, price.Recurring)
	assert.Equal(t, "month", price.Recurring.Interval)
	assert.Empty(t, price.CustomLabel)
}

func TestRenderStripe_CustomLabelTierHasNoAmount(t *testing.T) {
	t.Parallel()

	v := domain.Variation{
		Headline: "Simple, transparent pricing",
		Tiers: []domain.PricingTier{
			{Name: "Enterprise", Price: domain.Price{CustomLabel: "Contact us"}},
		},
	}
	result := renderStripe(v)

	var table stripePricingTable
	require.NoError(t, json.Unmarshal([]byte(result.Content), &table))
	require.Len(t, table.Prices, 1)

	price := table.Prices[0]
	assert.Equal(t, "Contact us", price.CustomLabel)
	assert.Nil(t, price.UnitAmount)
	assert.Empty(t, price.Currency)
	assert.Nil(t, price.Recurring)
}

func TestRenderStripe_OneTimeIntervalHasNoRecurring(t *testing.T) {
	t.Parallel()

	v := domain.Variation{
		Headline: "Simple, transparent pricing",
		Tiers: []domain.PricingTier{
			{Name: "Lifetime", Price: domain.Price{AmountMinorUnits: 9900, Currency: "USD", Interval: domain.IntervalOneTime}},
		},
	}
	result := renderStripe(v)

	var table stripePricingTable
	require.NoError(t, json.Unmarshal([]byte(result.Content), &table))
	assert.Nil(t, table.Prices[0].Recurring)
}
