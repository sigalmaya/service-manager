/*
 * Copyright 2018 The Service Manager Authors
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 */

package util

import "fmt"

type Translator interface {
	Translate(input string) (string, error)
}

type Operation interface {
	fmt.Stringer
	IsMultivalue() bool
}

type FilterStatement struct {
	LeftOp  string
	Op      Operation
	RightOp []string
}

func (fs FilterStatement) Validate() error {
	if len(fs.RightOp) > 1 && !fs.Op.IsMultivalue() {
		return fmt.Errorf("multiple values received for single value operation")
	}

	return nil
}

func NewFilterStatement(leftOp string, op Operation, rightOp []string) FilterStatement {
	return FilterStatement{
		LeftOp:  leftOp,
		Op:      op,
		RightOp: rightOp,
	}
}
