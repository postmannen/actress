// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package actress

import (
	"bytes"
	"sync"
)

type Buffer struct {
	buffer bytes.Buffer
	mu     sync.Mutex
}

func NewBuffer() *Buffer {
	b := Buffer{}
	return &b
}

func (bu *Buffer) Read(p []byte) (int, error) {
	bu.mu.Lock()
	defer bu.mu.Unlock()
	return bu.buffer.Read(p)
}

func (bu *Buffer) Write(b []byte) (int, error) {
	bu.mu.Lock()
	defer bu.mu.Unlock()
	return bu.buffer.Write(b)
}
