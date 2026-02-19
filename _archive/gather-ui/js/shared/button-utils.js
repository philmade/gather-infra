/**
 * Button Utilities — Loading state management
 * Replaces duplicate try/finally disable/enable patterns
 */

/**
 * Wrap an async action with button loading state
 * @param {string|HTMLElement} btn - Button ID or element
 * @param {string} loadingText - Text to show while loading
 * @param {Function} asyncFn - Async function to execute
 */
async function withLoadingButton(btn, loadingText, asyncFn) {
    const el = typeof btn === 'string' ? document.getElementById(btn) : btn;
    if (!el) return asyncFn();

    const originalText = el.textContent;
    const originalHtml = el.innerHTML;
    const wasDisabled = el.disabled;

    el.disabled = true;
    el.textContent = loadingText;

    try {
        return await asyncFn();
    } finally {
        el.disabled = wasDisabled;
        el.innerHTML = originalHtml;
    }
}

window.withLoadingButton = withLoadingButton;
