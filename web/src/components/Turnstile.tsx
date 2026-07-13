import { useEffect, useRef, useState } from 'react';

// Cloudflare Turnstile injects a global `turnstile` object once the script
// loads. We lazily append the script exactly once per page.
const TURNSTILE_SCRIPT = 'https://challenges.cloudflare.com/turnstile/v0/api.js';
let scriptAppendPromise: Promise<void> | null = null;

function loadTurnstileScript(): Promise<void> {
  if (scriptAppendPromise) return scriptAppendPromise;
  scriptAppendPromise = new Promise<void>((resolve) => {
    if (window.turnstile) {
      resolve();
      return;
    }
    const s = document.createElement('script');
    s.src = TURNSTILE_SCRIPT;
    s.async = true;
    s.defer = true;
    s.onload = () => resolve();
    s.onerror = () => resolve(); // don't block login if CDN fails
    document.head.appendChild(s);
  });
  return scriptAppendPromise;
}

// Minimal typing for the parts of the Turnstile API we use.
interface TurnstileRenderOptions {
  sitekey: string;
  callback: (token: string) => void;
  'expired-callback'?: () => void;
  theme?: 'light' | 'dark' | 'auto';
  size?: 'normal' | 'compact';
}
interface TurnstileInstance {
  render: (container: string | HTMLElement, options: TurnstileRenderOptions) => string;
  getResponse: (widgetId?: string) => string;
  reset: (widgetId?: string) => void;
  remove: (widgetId?: string) => void;
}
declare global {
  interface Window {
    turnstile?: TurnstileInstance;
  }
}

interface TurnstileProps {
  siteKey: string;
  onVerify: (token: string) => void;
  onExpire?: () => void;
  theme?: 'light' | 'dark' | 'auto';
  size?: 'normal' | 'compact';
  className?: string;
}

// Turnstile wraps Cloudflare's client-side CAPTCHA. It renders the challenge
// into a container div and reports the resulting token up via onVerify. Parent
// components read the token and include it with the login request.
export default function Turnstile({
  siteKey,
  onVerify,
  onExpire,
  theme = 'auto',
  size = 'normal',
  className,
}: TurnstileProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const widgetIdRef = useRef<string | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let cancelled = false;
    loadTurnstileScript().then(() => {
      if (cancelled || !containerRef.current || !window.turnstile) return;
      const id = window.turnstile.render(containerRef.current, {
        sitekey: siteKey,
        callback: onVerify,
        'expired-callback': () => {
          widgetIdRef.current = null;
          onExpire?.();
        },
        theme,
        size,
      });
      widgetIdRef.current = id;
      setReady(true);
    });
    return () => {
      cancelled = true;
      if (widgetIdRef.current && window.turnstile) {
        try {
          window.turnstile.remove(widgetIdRef.current);
        } catch {
          /* ignore */
        }
        widgetIdRef.current = null;
      }
    };
  }, [siteKey, onVerify, onExpire, theme, size]);

  // Expose a way for the parent to reset the widget (e.g. after a failed attempt)
  useEffect(() => {
    if (ready && widgetIdRef.current && window.turnstile) {
      // no-op: widget mounted
    }
  }, [ready]);

  return <div ref={containerRef} className={className} />;
}

// Reset a rendered widget so the user can re-solve the challenge.
export function resetTurnstile(): void {
  if (window.turnstile) {
    try {
      window.turnstile.reset();
    } catch {
      /* ignore */
    }
  }
}
