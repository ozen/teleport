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

package athena

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snsTypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
)

// fakeQueue is used to fake SNS+SQS combination on AWS.
type fakeQueue struct {
	// publishErrors is chain of error returns on Publish method.
	// Errors are returned from start to end and removed, one-by-one, on each
	// invocation of the Publish method.
	// If the slice is empty, Publish runs normally.
	publishErrors []error
	mu            sync.Mutex
	msgs          []fakeQueueMessage
}

type fakeQueueMessage struct {
	payload    string
	attributes map[string]snsTypes.MessageAttributeValue
}

func newFakeQueue() *fakeQueue {
	return &fakeQueue{}
}

func (f *fakeQueue) Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.publishErrors) > 0 {
		err := f.publishErrors[0]
		f.publishErrors = f.publishErrors[1:]
		return nil, err
	}
	f.msgs = append(f.msgs, fakeQueueMessage{
		payload:    *params.Message,
		attributes: params.MessageAttributes,
	})
	return nil, nil
}

func (f *fakeQueue) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	msgs := f.dequeue()
	if len(msgs) == 0 {
		return &sqs.ReceiveMessageOutput{}, nil
	}
	out := make([]sqsTypes.Message, 0, 10)
	for _, msg := range msgs {
		out = append(out, sqsTypes.Message{
			Body:              aws.String(msg.payload),
			MessageAttributes: snsToSqsAttributes(msg.attributes),
			ReceiptHandle:     aws.String(uuid.NewString()),
		})
	}
	return &sqs.ReceiveMessageOutput{
		Messages: out,
	}, nil
}

func snsToSqsAttributes(in map[string]snsTypes.MessageAttributeValue) map[string]sqsTypes.MessageAttributeValue {
	if in == nil {
		return nil
	}
	out := map[string]sqsTypes.MessageAttributeValue{}
	for k, v := range in {
		out[k] = sqsTypes.MessageAttributeValue{
			DataType:    v.DataType,
			StringValue: v.StringValue,
		}
	}
	return out
}

func (f *fakeQueue) dequeue() []fakeQueueMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	batchSize := 10
	if len(f.msgs) == 0 {
		return nil
	}
	if len(f.msgs) < batchSize {
		batchSize = len(f.msgs)
	}
	items := f.msgs[:batchSize]
	f.msgs = f.msgs[batchSize:]
	return items
}
