/**
 * Copyright 2019 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/xmidt-org/bascule"
)

func CreateValidCapabilityCheck(firstPiece, secondPiece, thirdPiece, acceptAllMethod string) func(context.Context, []interface{}) error {
	return func(ctx context.Context, vals []interface{}) error {
		if len(vals) == 0 {
			return errors.New("expected at least one value")
		}

		auth, ok := bascule.FromContext(ctx)
		if !ok {
			return errors.New("couldn't get request info")
		}
		reqVal := auth.Request

		for _, val := range vals {
			str, ok := val.(string)
			if !ok {
				return errors.New("expected value to be a string")
			}
			if len(str) == 0 {
				return errors.New("expected string to be nonempty")
			}
			pieces := strings.Split(str, ":")
			if len(pieces) != 5 {
				return fmt.Errorf("malformed string: [%v]", str)
			}
			method := pieces[4]
			if method != acceptAllMethod && method != strings.ToLower(reqVal.Method) {
				continue
			}
			if pieces[0] != firstPiece || pieces[1] != secondPiece || pieces[2] != thirdPiece {
				continue
			}
			matched, err := regexp.MatchString(pieces[3], reqVal.URL)
			if err != nil {
				continue
			}
			if matched {
				return nil
			}
		}
		return errors.New("no valid capability for endpoint")
	}
}
