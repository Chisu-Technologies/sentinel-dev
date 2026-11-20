# Your own MCP servers

The Sentinel that `sentinel up` runs on your machine can give its agents the tools of
any [MCP](https://modelcontextprotocol.io) server you choose: one you run on this
computer, one on your network, or a hosted one such as Linear's, Sentry's or Composio
Connect. Everything happens on your machine: Sentinel's API calls the server, and no
service of ours is involved.

## Add a server

1. Open **Settings > External tools** and choose **Add MCP server**.
2. Give it a name, its address, and, if the server asks for one, a token.
   - A server on this computer: `http://localhost:3000/mcp` works. Sentinel stores it
     as `host.docker.internal`, the name its containers give this machine.
   - Plain `http://` only reaches this computer or a private address. Anything on the
     internet must be `https://`, so a token never crosses it in clear.
   - The token is encrypted in Sentinel's database and never shown again. It is sent
     as `Authorization: Bearer <token>`, unless you name another header.
3. **Check server** asks the server for its tools. Tools the server marks as
   read-only start on; tools that may change data start off until you turn each one on.
4. **Save server**.

The card then shows the server's **name in a contract**, for example `linear`.

## Let a task use it

An agent reaches a server only when the task's execution contract allows it. In
**Settings > Execution**, give the role the two external tool capabilities and
name the server:

```json
"required_capabilities": ["...", "external_tool_search", "external_tool_execute"],
"external_tools": {
  "allowed_toolkits": ["linear"],
  "allowed_actions": ["LINEAR_*"],
  "max_calls": 20
}
```

- `allowed_toolkits` names the servers, by their name in a contract.
- `allowed_actions` names the tools: `LINEAR_*` is every tool of `linear` you turned on,
  `LINEAR_LIST_ISSUES` only that one. A tool's action is the server's name and the
  tool's own name in capitals, with `_` for anything that is not a letter or a digit.
- `max_calls` is how many searches and calls one run may make.

A tool you turned off stays off whatever the contract says. Every search and call goes
through Sentinel, counts against `max_calls` and is recorded with the run.

## Composio Connect

[Composio Connect](https://composio.dev) is one MCP server that reaches the apps you
connect in your Composio account (Gmail, Slack, GitHub and many more).

1. In Composio's dashboard, create a consumer API key for an AI client.
2. Add a server named `Composio` with the address `https://connect.composio.dev/mcp`,
   your consumer key as the token, and `x-consumer-api-key` as the header.
3. The first time an app is needed, Composio gives you a link to authorize it in your
   browser. That account lives in Composio, under your Composio plan.

## Limits

- Up to 20 servers, and up to 200 tools from each.
- A server's answer may be at most 1 MiB, and it has 30 seconds to give it.
- Sentinel speaks MCP's Streamable HTTP transport. A server that only offers the older
  HTTP+SSE transport (a separate `/sse` address) is refused as "not an MCP server".
- Tokens only for now: a server that requires its own OAuth sign-in cannot be added yet.
