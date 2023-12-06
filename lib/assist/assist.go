/*
 * Teleport
 * Copyright (C) 2023  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package assist

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gravitational/trace"
	"github.com/gravitational/trace/trail"
	"github.com/jonboulle/clockwork"
	"github.com/sashabaranov/go-openai"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/gravitational/teleport/api/gen/proto/go/assist/v1"
	pluginsv1 "github.com/gravitational/teleport/api/gen/proto/go/teleport/plugins/v1"
	"github.com/gravitational/teleport/lib/ai"
	"github.com/gravitational/teleport/lib/ai/model"
	"github.com/gravitational/teleport/lib/ai/model/output"
	"github.com/gravitational/teleport/lib/ai/model/tools"
	"github.com/gravitational/teleport/lib/ai/tokens"
)

// MessageType is a type of the Assist message.
type MessageType string

const (
	// MessageKindCommand is the type of Assist message that contains the command to execute.
	MessageKindCommand MessageType = "COMMAND"
	// MessageKindCommandResult is the type of Assist message that contains the command execution result.
	MessageKindCommandResult MessageType = "COMMAND_RESULT"
	// MessageKindAccessRequest is the type of Assist message that contains the access request.
	// Sent by the backend when it wants the frontend to display a prompt to the user.
	MessageKindAccessRequest MessageType = "ACCESS_REQUEST"
	// MessageKindAccessRequestCreated is a marker message to indicate that an access request was created.
	// Sent by the frontend to the backend to indicate that it was created to future loads of the conversation.
	MessageKindAccessRequestCreated MessageType = "ACCESS_REQUEST_CREATED"
	// MessageKindCommandResultSummary is the type of message that is optionally
	// emitted after a command and contains a summary of the command output.
	// This message is both sent after the command execution to the web UI,
	// and persisted in the conversation history.
	MessageKindCommandResultSummary MessageType = "COMMAND_RESULT_SUMMARY"
	// MessageKindUserMessage is the type of Assist message that contains the user message.
	MessageKindUserMessage MessageType = "CHAT_MESSAGE_USER"
	// MessageKindAssistantMessage is the type of Assist message that contains the assistant message.
	MessageKindAssistantMessage MessageType = "CHAT_MESSAGE_ASSISTANT"
	// MessageKindAssistantPartialMessage is the type of Assist message that contains the assistant partial message.
	MessageKindAssistantPartialMessage MessageType = "CHAT_PARTIAL_MESSAGE_ASSISTANT"
	// MessageKindAssistantPartialFinalize is the type of Assist message that ends the partial message stream.
	MessageKindAssistantPartialFinalize MessageType = "CHAT_PARTIAL_MESSAGE_ASSISTANT_FINALIZE"
	// MessageKindSystemMessage is the type of Assist message that contains the system message.
	MessageKindSystemMessage MessageType = "CHAT_MESSAGE_SYSTEM"
	// MessageKindError is the type of Assist message that is presented to user as information, but not stored persistently in the conversation. This can include backend error messages and the like.
	MessageKindError MessageType = "CHAT_MESSAGE_ERROR"
	// MessageKindProgressUpdate is the type of Assist message that contains a progress update.
	// A progress update starts a new "stage" and ends a previous stage if there was one.
	MessageKindProgressUpdate MessageType = "CHAT_MESSAGE_PROGRESS_UPDATE"
)

// PluginGetter is the minimal interface used by the chat to interact with the plugin service in the backend.
type PluginGetter interface {
	PluginsClient() pluginsv1.PluginServiceClient
}

// MessageService is the minimal interface used by the chat to interact with the Assist message service in the backend.
type MessageService interface {
	// GetAssistantMessages returns all messages with given conversation ID.
	GetAssistantMessages(ctx context.Context, req *assist.GetAssistantMessagesRequest) (*assist.GetAssistantMessagesResponse, error)

	// CreateAssistantMessage adds the message to the backend.
	CreateAssistantMessage(ctx context.Context, msg *assist.CreateAssistantMessageRequest) error
}

// Assist is the Teleport Assist client.
type Assist struct {
	client *ai.Client
	// clock is a clock used to generate timestamps.
	clock clockwork.Clock
}

// NewClient creates a new Assist client.
func NewClient(ctx context.Context, proxyClient PluginGetter,
	proxySettings any, openaiCfg *openai.ClientConfig) (*Assist, error) {

	client, err := getAssistantClient(ctx, proxyClient, proxySettings, openaiCfg)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &Assist{
		client: client,
		clock:  clockwork.NewRealClock(),
	}, nil
}

// Chat is a Teleport Assist chat.
type Chat struct {
	assist *Assist
	chat   *ai.Chat
	// assistService is the auth server client.
	assistService MessageService
	// ConversationID is the ID of the conversation.
	ConversationID string
	// Username is the username of the user who started the chat.
	Username string
	// potentiallyStaleHistory indicates messages might have been inserted into
	// the chat history and the messages should be re-fetched before attempting
	// the next completion.
	potentiallyStaleHistory bool
}

// NewChat creates a new Assist chat.
func (a *Assist) NewChat(ctx context.Context, assistService MessageService, toolContext *tools.ToolContext,
	conversationID string,
) (*Chat, error) {
	aichat := a.client.NewChat(toolContext)

	chat := &Chat{
		assist:                  a,
		assistService:           assistService,
		chat:                    aichat,
		ConversationID:          conversationID,
		Username:                toolContext.User,
		potentiallyStaleHistory: false,
	}

	if err := chat.loadMessages(ctx); err != nil {
		return nil, trace.Wrap(err)
	}

	return chat, nil
}

// LightweightChat is a Teleport Assist chat that doesn't store the history
// of the conversation.
type LightweightChat struct {
	assist *Assist
	chat   *ai.Chat
}

// NewLightweightChat creates a new Assist chat what doesn't store the history
// of the conversation.
func (a *Assist) NewLightweightChat(username string) (*LightweightChat, error) {
	aichat := a.client.NewCommand(username)
	return &LightweightChat{
		assist: a,
		chat:   aichat,
	}, nil
}

func (a *Assist) NewSSHCommand(username string) (*ai.Chat, error) {
	return a.client.NewCommand(username), nil
}

// GenerateSummary generates a summary for the given message.
func (a *Assist) GenerateSummary(ctx context.Context, message string) (string, error) {
	return a.client.Summary(ctx, message)
}

// RunTool runs a model tool without an ai.Chat.
func (a *Assist) RunTool(ctx context.Context, onMessage onMessageFunc, toolName, userInput string, toolContext *tools.ToolContext,
) (*tokens.TokenCount, error) {
	message, tc, err := a.client.RunTool(ctx, toolContext, toolName, userInput)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	switch message := message.(type) {
	case *output.Message:
		if err := onMessage(MessageKindAssistantMessage, []byte(message.Content), a.clock.Now().UTC()); err != nil {
			return nil, trace.Wrap(err)
		}
	case *output.GeneratedCommand:
		if err := onMessage(MessageKindCommand, []byte(message.Command), a.clock.Now().UTC()); err != nil {
			return nil, trace.Wrap(err)
		}
	case *output.StreamingMessage:
		if err := func() error {
			var text strings.Builder
			defer onMessage(MessageKindAssistantPartialFinalize, nil, a.clock.Now().UTC())
			for part := range message.Parts {
				text.WriteString(part)

				if err := onMessage(MessageKindAssistantPartialMessage, []byte(part), a.clock.Now().UTC()); err != nil {
					return trace.Wrap(err)
				}
			}
			return nil
		}(); err != nil {
			return nil, trace.Wrap(err)
		}
	default:
		return nil, trace.Errorf("Unexpected message type: %T", message)
	}

	return tc, nil
}

// GenerateCommandSummary summarizes the output of a command executed on one or
// many nodes. The conversation history is also sent into the prompt in order
// to gather context and know what information is relevant in the command output.
func (a *Assist) GenerateCommandSummary(ctx context.Context, messages []*assist.AssistantMessage, output map[string][]byte) (string, *tokens.TokenCount, error) {
	// Create system prompt
	modelMessages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: model.PromptSummarizeCommand},
	}

	// Load context back into prompt
	for _, message := range messages {
		role := kindToRole(MessageType(message.Type))
		if role != "" && role != openai.ChatMessageRoleSystem {
			payload, err := formatMessagePayload(message)
			if err != nil {
				return "", nil, trace.Wrap(err)
			}
			modelMessages = append(modelMessages, openai.ChatCompletionMessage{Role: role, Content: payload})
		}
	}
	return a.client.CommandSummary(ctx, modelMessages, output)
}

// reloadMessages clears the chat history and reloads the messages from the database.
func (c *Chat) reloadMessages(ctx context.Context) error {
	c.chat.Clear()
	return c.loadMessages(ctx)
}

// ClassifyMessage takes a user message, a list of categories, and uses the AI
// mode as a zero-shot classifier. It returns an error if the classification
// result is not a valid class.
func (a *Assist) ClassifyMessage(ctx context.Context, message string, classes map[string]string) (string, error) {
	category, err := a.client.ClassifyMessage(ctx, message, classes)
	if err != nil {
		return "", trace.Wrap(err)
	}

	cleanedCategory := strings.ToLower(strings.Trim(category, ". "))
	if _, ok := classes[cleanedCategory]; ok {
		return cleanedCategory, nil
	}

	return "", trace.CompareFailed("classification failed, category '%s' is not a valid classes", cleanedCategory)
}

// loadMessages loads the messages from the database.
func (c *Chat) loadMessages(ctx context.Context) error {
	// existing conversation, retrieve old messages
	messages, err := c.assistService.GetAssistantMessages(ctx, &assist.GetAssistantMessagesRequest{
		ConversationId: c.ConversationID,
		Username:       c.Username,
	})
	if err != nil {
		return trace.Wrap(err)
	}

	// restore conversation context.
	for _, msg := range messages.GetMessages() {
		role := kindToRole(MessageType(msg.Type))
		if role != "" {
			payload, err := formatMessagePayload(msg)
			if err != nil {
				return trace.Wrap(err)
			}
			c.chat.Insert(role, payload)
		}
	}

	// Mark the history as fresh.
	c.potentiallyStaleHistory = false

	return nil
}

// IsNewConversation returns true if the conversation has no messages yet.
func (c *Chat) IsNewConversation() bool {
	return len(c.chat.GetMessages()) == 1
}

// getAssistantClient returns the OpenAI client created base on Teleport Plugin information
// or the static token configured in YAML.
func getAssistantClient(ctx context.Context, proxyClient PluginGetter,
	proxySettings any, openaiCfg *openai.ClientConfig,
) (*ai.Client, error) {
	apiKey, err := getOpenAITokenFromDefaultPlugin(ctx, proxyClient)
	if err == nil {
		return ai.NewClient(apiKey), nil
	} else if !trace.IsNotFound(err) && !trace.IsNotImplemented(err) {
		// We ignore 2 types of errors here.
		// Unimplemented may be raised by the OSS server,
		// as PluginsService does not exist there yet.
		// NotFound means plugin does not exist,
		// in which case we should fall back on the static token configured in YAML.
		log.WithError(err).Error("Unexpected error fetching default OpenAI plugin")
	}

	// If the default plugin is not configured, try to get the token from the proxy settings.
	keyGetter, found := proxySettings.(interface{ GetOpenAIAPIKey() string })
	if !found {
		return nil, trace.Errorf("GetOpenAIAPIKey is not implemented on %T", proxySettings)
	}

	apiKey = keyGetter.GetOpenAIAPIKey()
	if apiKey == "" {
		return nil, trace.Errorf("OpenAI API key is not set")
	}

	// Allow using the passed config if passed.
	// In this case, apiKey is ignored, the one from the OpenAI config is used.
	if openaiCfg != nil {
		return ai.NewClientFromConfig(*openaiCfg), nil
	}
	return ai.NewClient(apiKey), nil
}

// onMessageFunc is a function that is called when a message is received.
type onMessageFunc func(kind MessageType, payload []byte, createdTime time.Time) error

// RecordMessage is used to record out-of-band messages such as hidden acknowledgements.
func (c *Chat) RecordMesssage(ctx context.Context, kind MessageType, payload string) error {
	switch kind {
	case MessageKindAccessRequestCreated:
		protoMsg := &assist.CreateAssistantMessageRequest{
			ConversationId: c.ConversationID,
			Username:       c.Username,
			Message: &assist.AssistantMessage{
				Type:        string(MessageKindAssistantMessage),
				Payload:     payload,
				CreatedTime: timestamppb.New(c.assist.clock.Now().UTC()),
			},
		}

		if err := c.assistService.CreateAssistantMessage(ctx, protoMsg); err != nil {
			return trace.Wrap(err)
		}
	default:
		return trace.BadParameter("unsupported marker message kind: %v", kind)
	}

	return nil
}

// ProcessComplete processes the completion request and returns the number of tokens used.
func (c *Chat) ProcessComplete(ctx context.Context, onMessage onMessageFunc, userInput string,
) (*tokens.TokenCount, error) {
	progressUpdates := func(update *model.AgentAction) {
		payload, err := json.Marshal(update)
		if err != nil {
			log.WithError(err).Debugf("Failed to marshal progress update: %v", update)
			return
		}

		if err := onMessage(MessageKindProgressUpdate, payload, c.assist.clock.Now().UTC()); err != nil {
			log.WithError(err).Debugf("Failed to send progress update: %v", update)
			return
		}
	}

	// If data might have been inserted into the chat history, we want to
	// refresh and get the latest data before querying the model.
	if c.potentiallyStaleHistory {
		if err := c.reloadMessages(ctx); err != nil {
			return nil, trace.Wrap(err)
		}
	}

	// query the assistant and fetch an answer
	message, tokenCount, err := c.chat.Complete(ctx, userInput, progressUpdates)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// write the user message to persistent storage and the chat structure
	c.chat.Insert(openai.ChatMessageRoleUser, userInput)

	// Do not write empty messages to the database.
	if userInput != "" {
		if err := c.assistService.CreateAssistantMessage(ctx, &assist.CreateAssistantMessageRequest{
			Message: &assist.AssistantMessage{
				Type:        string(MessageKindUserMessage),
				Payload:     userInput, // TODO(jakule): Sanitize the payload
				CreatedTime: timestamppb.New(c.assist.clock.Now().UTC()),
			},
			ConversationId: c.ConversationID,
			Username:       c.Username,
		}); err != nil {
			return nil, trace.Wrap(err)
		}
	}

	switch message := message.(type) {
	case *output.Message:
		c.chat.Insert(openai.ChatMessageRoleAssistant, message.Content)

		// write an assistant message to persistent storage
		protoMsg := &assist.CreateAssistantMessageRequest{
			ConversationId: c.ConversationID,
			Username:       c.Username,
			Message: &assist.AssistantMessage{
				Type:        string(MessageKindAssistantMessage),
				Payload:     message.Content,
				CreatedTime: timestamppb.New(c.assist.clock.Now().UTC()),
			},
		}

		if err := c.assistService.CreateAssistantMessage(ctx, protoMsg); err != nil {
			return nil, trace.Wrap(err)
		}

		if err := onMessage(MessageKindAssistantMessage, []byte(message.Content), c.assist.clock.Now().UTC()); err != nil {
			return nil, trace.Wrap(err)
		}
	case *output.StreamingMessage:
		var text strings.Builder
		defer onMessage(MessageKindAssistantPartialFinalize, nil, c.assist.clock.Now().UTC())
		for part := range message.Parts {
			text.WriteString(part)

			if err := onMessage(MessageKindAssistantPartialMessage, []byte(part), c.assist.clock.Now().UTC()); err != nil {
				return nil, trace.Wrap(err)
			}
		}

		// write an assistant message to memory and persistent storage
		textS := text.String()
		c.chat.Insert(openai.ChatMessageRoleAssistant, textS)
		protoMsg := &assist.CreateAssistantMessageRequest{
			ConversationId: c.ConversationID,
			Username:       c.Username,
			Message: &assist.AssistantMessage{
				Type:        string(MessageKindAssistantMessage),
				Payload:     textS,
				CreatedTime: timestamppb.New(c.assist.clock.Now().UTC()),
			},
		}

		if err := c.assistService.CreateAssistantMessage(ctx, protoMsg); err != nil {
			return nil, trace.Wrap(err)
		}
	case *output.CompletionCommand:
		payloadJson, err := json.Marshal(message)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		msg := &assist.CreateAssistantMessageRequest{
			ConversationId: c.ConversationID,
			Username:       c.Username,
			Message: &assist.AssistantMessage{
				Type:        string(MessageKindCommand),
				Payload:     string(payloadJson),
				CreatedTime: timestamppb.New(c.assist.clock.Now().UTC()),
			},
		}

		if err := c.assistService.CreateAssistantMessage(ctx, msg); err != nil {
			return nil, trace.Wrap(err)
		}

		if err := onMessage(MessageKindCommand, payloadJson, c.assist.clock.Now().UTC()); nil != err {
			return nil, trace.Wrap(err)
		}
		// As we emitted a command suggestion, the user might have run it. If
		// the command ran, a summary could have been inserted in the backend.
		// To take this command summary into account, we note the history might
		// be stale.
		c.potentiallyStaleHistory = true
	case *output.AccessRequest:
		payloadJson, err := json.Marshal(message)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		msg := &assist.CreateAssistantMessageRequest{
			ConversationId: c.ConversationID,
			Username:       c.Username,
			Message: &assist.AssistantMessage{
				Type:        string(MessageKindAccessRequest),
				Payload:     string(payloadJson),
				CreatedTime: timestamppb.New(c.assist.clock.Now().UTC()),
			},
		}

		if err := c.assistService.CreateAssistantMessage(ctx, msg); err != nil {
			return nil, trace.Wrap(err)
		}

		if err := onMessage(MessageKindAccessRequest, payloadJson, c.assist.clock.Now().UTC()); nil != err {
			return nil, trace.Wrap(err)
		}
	default:
		return nil, trace.Errorf("unknown message type: %T", message)
	}

	return tokenCount, nil
}

// ProcessComplete processes a user message and returns the assistant's response.
func (c *LightweightChat) ProcessComplete(ctx context.Context, onMessage onMessageFunc, userInput string,
) (*tokens.TokenCount, error) {
	progressUpdates := func(update *model.AgentAction) {
		payload, err := json.Marshal(update)
		if err != nil {
			log.WithError(err).Debugf("Failed to marshal progress update: %v", update)
			return
		}

		if err := onMessage(MessageKindProgressUpdate, payload, c.assist.clock.Now().UTC()); err != nil {
			log.WithError(err).Debugf("Failed to send progress update: %v", update)
			return
		}
	}

	message, tokenCount, err := c.chat.Reply(ctx, userInput, progressUpdates)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	c.chat.Insert(openai.ChatMessageRoleUser, userInput)

	switch message := message.(type) {
	case *output.Message:
		c.chat.Insert(openai.ChatMessageRoleAssistant, message.Content)
		if err := onMessage(MessageKindAssistantMessage, []byte(message.Content), c.assist.clock.Now().UTC()); err != nil {
			return nil, trace.Wrap(err)
		}
	case *output.GeneratedCommand:
		c.chat.Insert(openai.ChatMessageRoleAssistant, message.Command)
		if err := onMessage(MessageKindCommand, []byte(message.Command), c.assist.clock.Now().UTC()); err != nil {
			return nil, trace.Wrap(err)
		}
	case *output.StreamingMessage:
		if err := func() error {
			var text strings.Builder
			defer onMessage(MessageKindAssistantPartialFinalize, nil, c.assist.clock.Now().UTC())
			for part := range message.Parts {
				text.WriteString(part)

				if err := onMessage(MessageKindAssistantPartialMessage, []byte(part), c.assist.clock.Now().UTC()); err != nil {
					return trace.Wrap(err)
				}
			}
			c.chat.Insert(openai.ChatMessageRoleAssistant, text.String())
			return nil
		}(); err != nil {
			return nil, trace.Wrap(err)
		}
	default:
		return nil, trace.Errorf("Unexpected message type: %T", message)
	}

	return tokenCount, nil
}

func getOpenAITokenFromDefaultPlugin(ctx context.Context, proxyClient PluginGetter) (string, error) {
	// Try retrieving credentials from the plugin resource first
	openaiPlugin, err := proxyClient.PluginsClient().GetPlugin(ctx, &pluginsv1.GetPluginRequest{
		Name:        "openai-default",
		WithSecrets: true,
	})
	if err != nil {
		return "", trail.FromGRPC(err)
	}

	creds := openaiPlugin.Credentials.GetBearerToken()
	if creds == nil {
		return "", trace.BadParameter("malformed credentials")
	}

	return creds.Token, nil
}

// kindToRole converts a message kind to an OpenAI role.
func kindToRole(kind MessageType) string {
	switch kind {
	case MessageKindUserMessage:
		return openai.ChatMessageRoleUser
	case MessageKindAssistantMessage:
		return openai.ChatMessageRoleAssistant
	case MessageKindSystemMessage:
		return openai.ChatMessageRoleSystem
	case MessageKindCommandResultSummary:
		return openai.ChatMessageRoleUser
	default:
		return ""
	}
}

// formatMessagePayload generates the OpemAI message payload corresponding to
// an Assist message. Most Assist message payloads can be converted directly,
// but some payloads are JSON-formatted and must be processed before being
// passed to the model.
func formatMessagePayload(message *assist.AssistantMessage) (string, error) {
	switch MessageType(message.GetType()) {
	case MessageKindCommandResultSummary:
		var summary CommandExecSummary
		err := json.Unmarshal([]byte(message.GetPayload()), &summary)
		if err != nil {
			return "", trace.Wrap(err)
		}
		return summary.String(), nil
	default:
		return message.GetPayload(), nil
	}
}
