package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("github.com/FarelRA/comix/internal/imagegen")

type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type ImageResult struct {
	Image         image.Image
	RevisedPrompt string
	Usage         TokenUsage
}

type Client struct {
	apiKey     string
	model      string
	quality    string
	baseURL    string
	maxRetries int
	httpClient *http.Client
	once       sync.Once
	client     openai.Client
}

func NewClient(apiKey, model, quality string) *Client {
	return &Client{
		apiKey:  apiKey,
		model:   model,
		quality: quality,
	}
}

func (c *Client) getClient() *openai.Client {
	c.once.Do(func() {
		opts := []option.RequestOption{
			option.WithAPIKey(c.apiKey),
		}
		if c.baseURL != "" {
			opts = append(opts, option.WithBaseURL(c.baseURL))
		}
		if c.httpClient != nil {
			opts = append(opts, option.WithHTTPClient(c.httpClient))
		}
		if c.maxRetries > 0 {
			opts = append(opts, option.WithMaxRetries(c.maxRetries))
		}
		cl := openai.NewClient(opts...)
		c.client = cl
	})
	return &c.client
}

func (c *Client) WithHTTPClient(hc *http.Client) *Client {
	c.httpClient = hc
	return c
}

func (c *Client) WithMaxRetries(n int) *Client {
	c.maxRetries = n
	return c
}

func (c *Client) WithBaseURL(url string) *Client {
	c.baseURL = url
	return c
}

func (c *Client) Generate(ctx context.Context, prompt, size string) (*ImageResult, error) {
	ctx, span := tracer.Start(ctx, "image.generate")
	defer span.End()
	resp, err := c.getClient().Images.Generate(ctx, openai.ImageGenerateParams{
		Model:   openai.ImageModel(c.model),
		Prompt:  prompt,
		N:       openai.Int(1),
		Quality: openai.ImageGenerateParamsQuality(c.quality),
		Size:    openai.ImageGenerateParamsSize(size),
	})
	if err != nil {
		return nil, fmt.Errorf("image generation: %w", err)
	}
	return parseResponse(resp, c.httpClient)
}

func (c *Client) GenerateWithReferences(ctx context.Context, prompt, size string, references ...image.Image) (*ImageResult, error) {
	ctx, span := tracer.Start(ctx, "image.generate_with_references")
	defer span.End()
	readers := make([]io.Reader, 0, len(references))
	for _, img := range references {
		reader, err := imageToPNGReader(img)
		if err != nil {
			return nil, fmt.Errorf("encoding input image: %w", err)
		}
		readers = append(readers, reader)
	}
	params := openai.ImageEditParams{
		Image: openai.ImageEditParamsImageUnion{
			OfFileArray: readers,
		},
		Prompt:  prompt,
		N:       openai.Int(1),
		Model:   openai.ImageModel(c.model),
		Quality: openai.ImageEditParamsQuality(c.quality),
		Size:    openai.ImageEditParamsSize(size),
	}
	resp, err := c.getClient().Images.Edit(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("image edit: %w", err)
	}
	return parseResponse(resp, c.httpClient)
}

func parseResponse(resp *openai.ImagesResponse, hc *http.Client) (*ImageResult, error) {
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("image response has no data")
	}

	d := resp.Data[0]
	result := &ImageResult{
		RevisedPrompt: d.RevisedPrompt,
		Usage: TokenUsage{
			PromptTokens:     int(resp.Usage.InputTokens),
			CompletionTokens: int(resp.Usage.OutputTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		},
	}

	if d.B64JSON != "" {
		decoded, err := base64.StdEncoding.DecodeString(d.B64JSON)
		if err != nil {
			return nil, fmt.Errorf("decoding base64 image: %w", err)
		}
		img, _, err := image.Decode(bytes.NewReader(decoded))
		if err != nil {
			return nil, fmt.Errorf("decoding image: %w", err)
		}
		result.Image = img
	} else if d.URL != "" {
		if hc == nil {
			hc = http.DefaultClient
		}
		imgResp, err := hc.Get(d.URL)
		if err != nil {
			return nil, fmt.Errorf("downloading image from url: %w", err)
		}
		defer imgResp.Body.Close()
		if imgResp.StatusCode < 200 || imgResp.StatusCode >= 300 {
			return nil, fmt.Errorf("downloading image from url: unexpected status %d", imgResp.StatusCode)
		}
		contentType := imgResp.Header.Get("Content-Type")
		if contentType != "" && !strings.HasPrefix(contentType, "image/") {
			return nil, fmt.Errorf("downloading image from url: unexpected content type %q", contentType)
		}
		limited := io.LimitReader(imgResp.Body, 50<<20)
		img, _, err := image.Decode(limited)
		if err != nil {
			return nil, fmt.Errorf("decoding downloaded image: %w", err)
		}
		result.Image = img
	} else {
		return nil, fmt.Errorf("image data has no b64_json or url")
	}

	return result, nil
}

func imageToPNGReader(img image.Image) (io.Reader, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return &buf, nil
}
