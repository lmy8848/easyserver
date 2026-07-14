// Reset a rendered Turnstile widget so the user can re-solve the challenge.
export function resetTurnstile(): void {
  if (window.turnstile) {
    try {
      window.turnstile.reset();
    } catch {
      /* ignore */
    }
  }
}