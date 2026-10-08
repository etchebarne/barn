import "./index.css"
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"

import { App } from "@/app/app"
import { isDesktop } from "@/lib/desktop"
import { registerServiceWorker } from "@/lib/push"

const root = document.getElementById("root")
if (!root) throw new Error("Missing #root element")

// The desktop app can't receive Web Push; its notifications come from the realtime connection.
if (!isDesktop()) registerServiceWorker()

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
