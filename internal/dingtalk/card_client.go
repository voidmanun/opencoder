package dingtalk

import (
	"fmt"
	"sync"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dingtalkim "github.com/alibabacloud-go/dingtalk/im_1_0"
	dingtalkoauth2 "github.com/alibabacloud-go/dingtalk/oauth2_1_0"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/google/uuid"
)

type CardClient struct {
	clientID     string
	clientSecret string
	imClient     *dingtalkim.Client
	oauthClient  *dingtalkoauth2.Client

	// Token management with expiry
	accessToken string
	tokenExpiry time.Time
	tokenMu     sync.RWMutex
}

func NewCardClient(clientID, clientSecret string) (*CardClient, error) {
	config := &openapi.Config{}
	config.Protocol = tea.String("https")
	config.RegionId = tea.String("central")

	imClient, err := dingtalkim.NewClient(config)
	if err != nil {
		return nil, err
	}

	oauthClient, err := dingtalkoauth2.NewClient(config)
	if err != nil {
		return nil, err
	}

	return &CardClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		imClient:     imClient,
		oauthClient:  oauthClient,
	}, nil
}

func (c *CardClient) getAccessToken() (string, error) {
	// Fast path: token valid?
	c.tokenMu.RLock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		token := c.accessToken
		c.tokenMu.RUnlock()
		return token, nil
	}
	c.tokenMu.RUnlock()
	// Slow path: refresh token
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// Double-check under write lock
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	request := &dingtalkoauth2.GetAccessTokenRequest{
		AppKey:    tea.String(c.clientID),
		AppSecret: tea.String(c.clientSecret),
	}

	response, err := c.oauthClient.GetAccessToken(request)
	if err != nil {
		return "", err
	}

	c.accessToken = *response.Body.AccessToken
	// Set expiry to 5 minutes before actual expiry to be safe
	// DingTalk tokens are often valid for ~2 hours; we refresh earlier
	c.tokenExpiry = time.Now().Add(115 * time.Minute)

	return c.accessToken, nil
}

type SendCardOptions struct {
	ConversationID   string
	ConversationType string
	SenderStaffID    string
	CardBizID        string
	CardData         string
}

func (c *CardClient) SendCard(opts *SendCardOptions) (string, error) {
	accessToken, err := c.getAccessToken()
	if err != nil {
		return "", err
	}

	if opts.CardBizID == "" {
		opts.CardBizID = uuid.New().String()
	}

	request := &dingtalkim.SendRobotInteractiveCardRequest{
		CardTemplateId: tea.String("StandardCard"),
		CardBizId:      tea.String(opts.CardBizID),
		CardData:       tea.String(opts.CardData),
		RobotCode:      tea.String(c.clientID),
		PullStrategy:   tea.Bool(false),
	}

	headers := &dingtalkim.SendRobotInteractiveCardHeaders{
		XAcsDingtalkAccessToken: tea.String(accessToken),
	}

	if opts.ConversationType == "2" {
		request.SetOpenConversationId(opts.ConversationID)
	} else {
		receiver := fmt.Sprintf(`{"userId":"%s"}`, opts.SenderStaffID)
		request.SetSingleChatReceiver(receiver)
	}

	_, err = c.imClient.SendRobotInteractiveCardWithOptions(request, headers, &util.RuntimeOptions{})
	if err != nil {
		// Retry once on auth/token failure by refreshing token and retrying
		// Clear cached token and retry
		c.tokenMu.Lock()
		c.accessToken = ""
		c.tokenExpiry = time.Time{}
		c.tokenMu.Unlock()

		newToken, derr := c.getAccessToken()
		if derr == nil {
			headers.XAcsDingtalkAccessToken = tea.String(newToken)
			_, err = c.imClient.SendRobotInteractiveCardWithOptions(request, headers, &util.RuntimeOptions{})
		}
		if derr != nil {
			return "", derr
		}
		if err != nil {
			return "", err
		}
	}

	return opts.CardBizID, nil
}

type UpdateCardOptions struct {
	CardBizID string
	CardData  string
}

func (c *CardClient) UpdateCard(opts *UpdateCardOptions) error {
	accessToken, err := c.getAccessToken()
	if err != nil {
		return err
	}

	request := &dingtalkim.UpdateRobotInteractiveCardRequest{
		CardBizId: tea.String(opts.CardBizID),
		CardData:  tea.String(opts.CardData),
	}

	headers := &dingtalkim.UpdateRobotInteractiveCardHeaders{
		XAcsDingtalkAccessToken: tea.String(accessToken),
	}

	_, err = c.imClient.UpdateRobotInteractiveCardWithOptions(request, headers, &util.RuntimeOptions{})
	if err != nil {
		// Retry once on auth/token failure by refreshing token and retrying
		c.tokenMu.Lock()
		c.accessToken = ""
		c.tokenExpiry = time.Time{}
		c.tokenMu.Unlock()

		newToken, derr := c.getAccessToken()
		if derr == nil {
			headers.XAcsDingtalkAccessToken = tea.String(newToken)
			_, err = c.imClient.UpdateRobotInteractiveCardWithOptions(request, headers, &util.RuntimeOptions{})
		}
		if derr != nil {
			return derr
		}
	}
	return err
}
