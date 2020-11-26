// espoke
// Copyright © 2018 Barthelemy Vessemont
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, version 3 of the License.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"github.com/alecthomas/kong"
	"github.com/criteo-forks/espoke/cmd"
)

var CLI struct {
	/*espoke is a whitebox probing tool for Elasticsearch clusters.
	It completes the following actions :
	 * discover every ES clusters registered in Consul
	 * run an empty search query against every discovered indexes, data servers & clusters
	 * expose latency metrics with tags for clusters and nodes
	 * expose avaibility metrics with tags for clusters and nodes*/
	Serve cmd.ServeCmd `cmd help:"espoke is a whitebox probing tool for Elasticsearch clusters"`
}

func main() {
	ctx := kong.Parse(&CLI)
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
