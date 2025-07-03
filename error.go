// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

type serverErr struct {
	error
	statusCode int
}

func (s serverErr) StatusCode() int {
	return s.statusCode
}
