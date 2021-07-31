import { React } from "https://unpkg.com/react-dom@16.7.0/umd/react-dom.production.min.js"
import { ReactDOM } from "https://unpkg.com/react-dom@16.7.0/umd/react-dom.production.min.js"
import { htm } from "https://unpkg.com/htm@3.1.0/dist/htm.js"

const html = htm.bind(React.createElement)

const Route = {
  "/": React.lazy(() => import("./home.js")),
  "*": React.lazy(() => import("./index.js")),
}

ReactDOM.render(
  html`
    <${React.Suspense} fallback=${html`<div></div>`}>
      <${Route[location.pathname] || Route["*"]} />
    <//>
  `,
  document.body
)