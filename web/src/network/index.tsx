import classNames from "classnames"
import * as React from "react"
import * as rf from "reaflow"
import {
  hasLink,
  NodeData,
  removeNode,
  getEdgesByNode,
  EdgeData,
} from "reaflow"

interface NetworkProps {
  nodes: {
    id: string
    text: string
    ports: [
      {
        id: string
        side: "SOUTH"
      }
    ]
  }[]
}

export function Network(props: NetworkProps) {
  const [selectedIds, setSelectedIds] = React.useState<string[]>([])

  const [nodes, setNodes] = React.useState<NodeData[]>([
    {
      id: "in",
      text: "in",
      ports: [
        {
          id: "x",
          height: 10,
          width: 10,
          side: "SOUTH",
        },
      ],
    },
    {
      id: "multi",
      text: "multi",
      ports: [
        {
          id: "nums[0]",
          height: 10,
          width: 10,
          side: "NORTH",
        },
        {
          id: "nums[1]",
          height: 10,
          width: 10,
          side: "NORTH",
        },
        {
          id: "mul",
          height: 10,
          width: 10,
          side: "SOUTH",
        },
      ],
    },
    {
      id: "out",
      text: "out",
      ports: [
        {
          id: "y",
          height: 10,
          width: 10,
          side: "NORTH",
        },
      ],
    },
  ])

  const [edges, setEdges] = React.useState<EdgeData[]>([
    {
      id: "in.x-multi.nums[0]",
      from: "in",
      to: "multi",
      fromPort: "x",
      toPort: "nums[0]",
    },
    {
      id: "in.x-multi.nums[1]",
      from: "in",
      to: "multi",
      fromPort: "x",
      toPort: "nums[1]",
    },
    {
      id: "multi.mul-out.y",
      from: "multi",
      to: "out",
      fromPort: "mul",
      toPort: "y",
    },
  ])

  const [draggingPortId, setDraggingPortId] = React.useState("")

  return (
    <div
      style={{
        position: "absolute",
        left: 0,
        right: 0,
        top: 0,
        bottom: 0,
        background: "#171010",
      }}
    >
      <rf.Canvas
        // https://www.eclipse.org/elk/reference/options.html
        // layoutOptions={
        //   {
        //     // "elk.nodeLabels.placement": "INSIDE V_CENTER H_RIGHT",
        //     // "elk.algorithm": "org.eclipse.elk.layered",
        //     // "elk.direction": "DOWN",
        //     // nodeLayering: "INTERACTIVE",
        //     // "org.eclipse.elk.edgeRouting": "ORTHOGONAL",
        //     // "elk.layered.unnecessaryBendpoints": "true",
        //     // "elk.layered.spacing.edgeNodeBetweenLayers": "20",
        //     // "org.eclipse.elk.layered.nodePlacement.bk.fixedAlignment": "BALANCED",
        //     // "org.eclipse.elk.layered.cycleBreaking.strategy": "DEPTH_FIRST",
        //     // "org.eclipse.elk.insideSelfLoops.activate": "true",
        //     // separateConnectedComponents: "false",
        //     // "spacing.componentComponent": "20",
        //     // spacing: "25",
        //     // "spacing.nodeNodeBetweenLayers": "20",
        //   }
        // }
        // dragNode={console.log}
        // dragEdge={console.log}
        animated={false}
        nodes={nodes}
        edges={edges}
        selections={selectedIds}
        onCanvasClick={() => setSelectedIds([])}
        onNodeLinkCheck={(_, from, to) => !hasLink(edges, from, to)}
        onNodeLink={(_, fromNode, toNode) => {
          // TODO link ports, not nodes!
          setEdges([
            ...edges,
            {
              id: `${fromNode.id}-${toNode.id}`,
              from: fromNode.id,
              to: toNode.id,
            },
          ])
        }}
        edge={
          <rf.Edge
            onClick={(_, edge) => setSelectedIds([edge.id])}
            // onEnter={console.log}
            // onLeave={console.log}
            onRemove={(_, e) => {
              setEdges(edges.filter(edge => edge.id !== e.id))
            }}
            onAdd={console.log}
          />
        }
        // zoom={false}
        node={node => (
          <rf.Node
            className={classNames("node", {
              activeNode: node.id == selectedIds[0],
            })}
            style={{ transition: "none" }}
            dragType="port"
            // onEnter={(_, port) => console.log(port)}
            // onLeave={(_, port) => console.log(port)}
            onClick={(_, node) => setSelectedIds([node.id])}
            onRemove={(_event, node) => {
              const results = removeNode(nodes, edges, [node.id])
              setNodes(results.nodes)
              setEdges(results.edges)
            }}
            port={
              <rf.Port
                onClick={(_, port) => setSelectedIds([port.id])}
                // onEnter={(_, port) => console.log(port)}
                // onLeave={(_, port) => console.log(port)}
                onDragStart={(...a) => console.log("start", ...a)}
                onDragEnd={(...a) => console.log("end", ...a)}
                style={{
                  fill: "#5c3f9b",
                  stroke: "#000000",
                  strokeWidth: "1px",
                }}
                rx={10}
                ry={10}
              />
            }
            remove={<rf.Remove />}
          />
        )}
      />
    </div>
  )
}