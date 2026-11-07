import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { Toaster } from "sonner";
import "pretendard/dist/web/variable/pretendardvariable-dynamic-subset.css";
import "./index.css";
import App from "./App";
import { I18nProvider, initialLang, loadLang } from "./i18n";
import { ThemeProvider, initTheme, useTheme } from "./lib/theme";
import { TooltipProvider } from "./components/ui/overlay";

initTheme();
await loadLang(initialLang());

function Toasts() {
  const { resolved } = useTheme();
  return (
    <Toaster theme={resolved} position="bottom-right" closeButton richColors={false}
      toastOptions={{ classNames: { toast: "!rounded-[3px] !border-line !bg-surface !text-fg !shadow-pop !font-sans", description: "!text-fg-muted" } }} />
  );
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ThemeProvider>
      <I18nProvider>
        <TooltipProvider delayDuration={250}>
          <BrowserRouter>
            <App />
          </BrowserRouter>
          <Toasts />
        </TooltipProvider>
      </I18nProvider>
    </ThemeProvider>
  </React.StrictMode>,
);
