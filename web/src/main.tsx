import "./index.css"
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"

import { App } from "@/app/app"
import { registerServiceWorker } from "@/lib/push"

const root = document.getElementById("root")
if (!root) throw new Error("Missing #root element")

registerServiceWorker()

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
