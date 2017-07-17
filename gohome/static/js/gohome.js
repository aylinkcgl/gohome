function gohomeweb() {

    var router = {};
    var nodes = [];
    var links = [];

    var palette = {
        "gray": "#777",
        "lightGray": "#ACACA7",
        "blue": "#03f",
        "lightBlue": "#81F7D8",
        "violet": "#c0f",
        "pink": "#f69",
        "green": "#4d4",
        "lightGreen": "#8e8",
        "yellow": "#ff0",
        "orange": "#f90",
        "red": "#f30"
    }

    var colors = {
        installed: palette.green,
        uninstalled: palette.lightGreen,
        unreachable: palette.lightGray,
        wiredLink: palette.yellow,
        losslessWireless: palette.orange,
        unreachableNeighbour: palette.red,
        current: palette.pink,
        neighbour: palette.violet,
        other: palette.blue,
        selected: palette.blue,
        route: palette.gray
    }

    function initLegend() {
        for (id in colors) {
            d3.selectAll(".legend-" + id)
                .append("svg:svg")
                .attr("width", 10)
                .attr("height", 10)
                .attr("class", "legend-dot")
                .append("svg:circle")
                .attr("cx", 5).attr("cy", 5).attr("r", 5)
                .attr("stroke-width", 0)
                .attr("fill", colors[id]);
        }
    }

    function update_row(d, name, headers) {
        /* For each <tr> row, iterate through <th> headers
           and add <td> cells with values from the data. */
        var tr = d3.select(this);
        var row = tr.selectAll("td")
            .data(headers.map(function(h) {
                return d.value[h];
            }));
        row.exit().remove();
        row.enter().append("td");

        row.filter(function(d) {
                return !(d instanceof Array);
            })
            .text(function(d) {
                return d;
            });

        var toto = row.filter(function(d) {
            return (d instanceof Array);
            d3.select(tr.selectAll("td")[0][3]).remove();
        });

        toto.append("table")

        subheaders = ["PeerId", "Peid", "Leid"];
        var thead = toto.select("table").append("thead").append("tr").selectAll("th")
            .data(subheaders)
            .enter().append("th")
            .text(function(d) {
                return d;
            });

        var subrow = toto.select("table")
            .append("tbody")
            .selectAll("tr")
            .data(d.value["Peer"])
            .enter().append("tr");

        subrow.each(function(peer) {
            var tr = d3.select(this);
            var row = tr.selectAll("td")
                .data(subheaders.map(function(header) {
                    return peer[header];
                }))
                .enter().append("td")
                .text(function(d) {
                    return d;
                });
        });
    }

    function recompute_table(name, message) {
        var table = d3.select("#" + name);
        table.select("tr.loading").remove();
        var headers = [];
        table.selectAll("th").each(function(m) {
            headers.push(d3.select(this).text());
        });
        var rows = table.select("tbody").selectAll("tr")
            .data(d3.entries(message), function(d) {
                if (typeof d == 'undefined') return null;
                else return d.key;
            });
        rows.enter().append("tr");
        rows.exit().remove();
        rows.each(function(d) {
            update_row.call(this, d, name, headers);
        });
    };

    function handleUpdate(message) {
        var jsonObject = JSON.parse(message.data);
        if (jsonObject != null) {
            if (jsonObject[0].Types == "neighbour") {
                recompute_table("neighbour", jsonObject);

            } else if (jsonObject[0].Types == "node") {
                recompute_table("node", jsonObject);
                nodes = [];
                links = [];
                for (i in jsonObject) {
                    router[i] = {
                        Id: jsonObject[i].Id
                    };
                    nodes.push(router[i]);
                }
                for (i in jsonObject) {
                    for (j in jsonObject[i].Peer) {
                        var peer = nodes.find(function(d) {
                            if (d.Id == jsonObject[i].Peer[j].PeerId) return d;
                        })
                        if (peer != 'undefined') {
                            links.push({
                                source: router[i],
                                target: peer
                            });
                        } else {
                            console.log("unknown peer");
                        }
                    }
                }

            }
        }
        display();
    }

    /* Update status message */
    function update_status(msg, good) {
        d3.select("#state").text(msg);
        if (good)
            d3.select("#state").style("background-color", palette.green);
        else
            d3.select("#state").style("background-color", palette.red);
    }

    function connect(server) {
        var sock = null;
        var ws = "ws://localhost:8000/websocket";

        sock = new WebSocket(ws);

        sock.onopen = function() {
            console.log("connected to " + ws);
            update_status("connected", true);
        }

        sock.onclose = function(e) {
            console.log("connection closed (" + e.code + ")");
            update_status("disconnected", false);
        }

        sock.onmessage = function(e) {
            handleUpdate(e);
        }
    }

    var vis;
    var width, height; /* display size */
    var w, h; /* virtual size */
    var force; /* force to coerce nodes */

    function setZoomLevel(x, y) {
        w = x;
        h = y;
        force.size([w, h]);
    }

    function initGraph() {
        /* Setup svg graph */
        width = 600;
        height = 400; /* display size */
        vis = d3.select("#fig")
            .insert("svg:svg", ".legend")
            .attr("width", width)
            .attr("height", height)
            .attr("stroke-width", "1.5px");
        force = d3.layout.force(); /* force to coerce nodes */
        force.charge(-1000); /* stronger repulsion enhances graph */
        force.on("tick", onTick);
    }

    function onTick() {
        vis.selectAll("circle.node")
            .attr("cx", function(d) {
                return (d.x);
            })
            .attr("cy", function(d) {
                return (d.y);
            });

        vis.selectAll("line.link")
            .attr("x1", function(d) {
                return (d.source.x);
            })
            .attr("y1", function(d) {
                return (d.source.y);
            })
            .attr("x2", function(d) {
                return (d.target.x);
            })
            .attr("y2", function(d) {
                return (d.target.y);
            });

        display();
    }

    function display() {

        force.nodes(nodes);
        force.links(links);
        force.linkDistance(120);
        force.start();

        /* Display routers */
        var node = vis.selectAll("circle.node")
            .data(nodes);
        node.enter().append("svg:circle")
            .attr("class", "node")
            .attr("r", 5)
            .attr("stroke", "white")
            .attr("id", function(d) {
                return "node-" + (d.Id);
            })
            .call(force.drag)
            .append("svg:title");
        node.exit().remove();

        vis.selectAll("circle.node").each(function(d) {
            d3.select(this).select("title")
                .text(d.Id);

        });

        /* Display link */
        var link = vis.selectAll("line.link")
            .data(links);
        link.enter().append("g")
            .attr("stroke", "black")
            .insert("line", "circle.node")
            .attr("class", "link")
            .attr("stroke-width", "1.5");
        link.exit().remove();
    }

    function init() {
        initLegend();
        initGraph();
        setZoomLevel(450, 400);
    }

    var gohomeweb = {}
    gohomeweb.init = init;
    gohomeweb.connect = connect;
    return gohomeweb;
}