package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/eXpressionist/handy-parser/internal/model"
)

type Extractor struct{ client *Client }

type PSPConfig struct {
	ProductID int64  `json:"product_id"`
	SKU       string `json:"sku"`
	Name      string `json:"name"`
}

func New(client *Client) *Extractor { return &Extractor{client: client} }

func ApplyKnownProfile(w model.Watch) model.Watch {
	if w.Kind != model.KindHTML || w.Selector != "" {
		return w
	}
	u, err := url.Parse(w.URL)
	if err != nil {
		return w
	}
	if strings.EqualFold(u.Hostname(), "gpc.ge") || strings.EqualFold(u.Hostname(), "www.gpc.ge") {
		w.Selector = `meta[name="product:price:amount"]`
		w.Attribute = "content"
		w.ValueType = model.ValuePrice
		if w.Currency == "" {
			w.Currency = "GEL"
		}
	}
	return w
}

func (e *Extractor) Observe(ctx context.Context, w model.Watch) (model.Observation, error) {
	w = ApplyKnownProfile(w)
	switch w.Kind {
	case model.KindHTML:
		return e.html(ctx, w)
	case model.KindPSP:
		return e.psp(ctx, w)
	default:
		return model.Observation{}, fmt.Errorf("unsupported extractor %q", w.Kind)
	}
}

func (e *Extractor) html(ctx context.Context, w model.Watch) (model.Observation, error) {
	body, _, err := e.client.Get(ctx, w.URL, nil)
	if err != nil {
		return model.Observation{}, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return model.Observation{}, fmt.Errorf("parse HTML: %w", err)
	}
	selection := doc.Find(w.Selector)
	if n := selection.Length(); n != 1 {
		return model.Observation{}, fmt.Errorf("selector matched %d elements; expected 1", n)
	}
	raw := ""
	if w.Attribute != "" {
		var ok bool
		raw, ok = selection.Attr(w.Attribute)
		if !ok {
			return model.Observation{}, fmt.Errorf("attribute %q is missing", w.Attribute)
		}
	} else {
		raw = selection.Text()
	}
	currency := w.Currency
	if w.ValueType == model.ValuePrice && currency == "" {
		currency = documentCurrency(doc)
	}
	return Normalize(raw, w.ValueType, currency)
}

func documentCurrency(doc *goquery.Document) string {
	selectors := []string{`meta[name="product:price:currency"]`, `meta[property="product:price:currency"]`, `meta[itemprop="priceCurrency"]`}
	for _, selector := range selectors {
		if value, ok := doc.Find(selector).First().Attr("content"); ok {
			value = strings.ToUpper(strings.TrimSpace(value))
			if len(value) >= 3 && len(value) <= 8 {
				return value
			}
		}
	}
	return ""
}

func (e *Extractor) ResolvePSP(ctx context.Context, rawURL string) (PSPConfig, model.Observation, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return PSPConfig{}, model.Observation{}, err
	}
	if !strings.EqualFold(u.Hostname(), "psp.ge") && !strings.EqualFold(u.Hostname(), "www.psp.ge") {
		return PSPConfig{}, model.Observation{}, fmt.Errorf("PSP adapter accepts only psp.ge URLs")
	}
	path := strings.TrimPrefix(u.EscapedPath(), "/")
	if path == "" {
		return PSPConfig{}, model.Observation{}, fmt.Errorf("product path is empty")
	}
	const q = `query urlResolver($url: String!) { urlResolver(url: $url) { id type relative_url } }`
	var response struct {
		Data struct {
			URLResolver *struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"urlResolver"`
		} `json:"data"`
		Errors []graphError `json:"errors"`
	}
	if err := e.graphql(ctx, q, map[string]any{"url": path}, &response); err != nil {
		return PSPConfig{}, model.Observation{}, err
	}
	if len(response.Errors) > 0 {
		return PSPConfig{}, model.Observation{}, fmt.Errorf("PSP GraphQL: %s", response.Errors[0].Message)
	}
	if response.Data.URLResolver == nil || response.Data.URLResolver.Type != "PRODUCT" {
		return PSPConfig{}, model.Observation{}, fmt.Errorf("URL does not resolve to a PSP product")
	}
	var id int64
	if _, err := fmt.Sscan(response.Data.URLResolver.ID, &id); err != nil || id <= 0 {
		return PSPConfig{}, model.Observation{}, fmt.Errorf("invalid PSP product ID")
	}
	return e.pspByID(ctx, id)
}

func (e *Extractor) psp(ctx context.Context, w model.Watch) (model.Observation, error) {
	var cfg PSPConfig
	if err := json.Unmarshal([]byte(w.AdapterConfig), &cfg); err != nil || cfg.ProductID <= 0 {
		return model.Observation{}, fmt.Errorf("invalid PSP adapter configuration")
	}
	actual, obs, err := e.pspByID(ctx, cfg.ProductID)
	if err == nil && (cfg.SKU == "" || actual.SKU == cfg.SKU) {
		return obs, nil
	}
	resolved, resolvedObs, resolveErr := e.ResolvePSP(ctx, w.URL)
	if resolveErr != nil {
		return model.Observation{}, fmt.Errorf("stored PSP product failed (%v); URL resolution failed (%v)", err, resolveErr)
	}
	if cfg.SKU != "" && resolved.SKU != cfg.SKU {
		return model.Observation{}, fmt.Errorf("PSP product SKU changed from %s to %s", cfg.SKU, resolved.SKU)
	}
	encoded, _ := json.Marshal(resolved)
	if resolvedObs.Metadata == nil {
		resolvedObs.Metadata = map[string]string{}
	}
	resolvedObs.Metadata["adapter_config"] = string(encoded)
	return resolvedObs, nil
}

func (e *Extractor) pspByID(ctx context.Context, id int64) (PSPConfig, model.Observation, error) {
	const q = `query productsByID($id: Int!) { productsByID(id: $id) { name sku stock_status price_range { maximum_price { final_price { value } regular_price { value } } } } }`
	var response struct {
		Data struct {
			Product *struct {
				Name       string `json:"name"`
				SKU        string `json:"sku"`
				Stock      string `json:"stock_status"`
				PriceRange struct {
					Maximum struct {
						Final struct {
							Value json.Number `json:"value"`
						} `json:"final_price"`
						Regular struct {
							Value json.Number `json:"value"`
						} `json:"regular_price"`
					} `json:"maximum_price"`
				} `json:"price_range"`
			} `json:"productsByID"`
		} `json:"data"`
		Errors []graphError `json:"errors"`
	}
	if err := e.graphql(ctx, q, map[string]any{"id": id}, &response); err != nil {
		return PSPConfig{}, model.Observation{}, err
	}
	if len(response.Errors) > 0 {
		return PSPConfig{}, model.Observation{}, fmt.Errorf("PSP GraphQL: %s", response.Errors[0].Message)
	}
	if response.Data.Product == nil {
		return PSPConfig{}, model.Observation{}, fmt.Errorf("PSP product %d not found", id)
	}
	p := response.Data.Product
	obs, err := Normalize(p.PriceRange.Maximum.Final.Value.String(), model.ValuePrice, "GEL")
	if err != nil {
		return PSPConfig{}, model.Observation{}, err
	}
	obs.Metadata = map[string]string{"sku": p.SKU, "name": p.Name, "stock": p.Stock, "regular_price": p.PriceRange.Maximum.Regular.Value.String()}
	return PSPConfig{ProductID: id, SKU: p.SKU, Name: p.Name}, obs, nil
}

type graphError struct {
	Message string `json:"message"`
}

func (e *Extractor) graphql(ctx context.Context, query string, variables any, dst any) error {
	vars, err := json.Marshal(variables)
	if err != nil {
		return err
	}
	u, _ := url.Parse("https://app.psp.ge/graphql")
	params := u.Query()
	params.Set("query", query)
	params.Set("variables", string(vars))
	params.Set("_lang", "ka")
	u.RawQuery = params.Encode()
	body, _, err := e.client.Get(ctx, u.String(), map[string]string{"store": "ka", "Accept": "application/json"})
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err = dec.Decode(dst); err != nil {
		return fmt.Errorf("decode PSP response: %w", err)
	}
	return nil
}
