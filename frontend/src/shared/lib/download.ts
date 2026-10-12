/** Triggers a browser download of an in-memory file. */
export function downloadText(fileName: string, content: string, mime = 'application/json'): void {
  downloadBlob(fileName, new Blob([content], { type: mime }));
}

export function downloadBlob(fileName: string, blob: Blob): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = fileName;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function fileNameFrom(disposition: string | null): string | null {
  if (!disposition) return null;
  const utf = /filename\*=UTF-8''([^;]+)/i.exec(disposition);
  if (utf) return decodeURIComponent(utf[1]);
  return /filename="?([^";]+)"?/i.exec(disposition)?.[1] ?? null;
}

/**
 * Downloads a file through fetch (works with the MSW mock worker and surfaces
 * API errors instead of navigating to an error page).
 */
export async function downloadFromUrl(url: string, fallbackName: string): Promise<void> {
  const res = await fetch(url);
  if (!res.ok) {
    let message = `HTTP ${res.status}`;
    try {
      const body = (await res.json()) as { error?: { message?: string } };
      message = body.error?.message ?? message;
    } catch {
      // non-JSON error body
    }
    throw new Error(message);
  }
  downloadBlob(fileNameFrom(res.headers.get('Content-Disposition')) ?? fallbackName, await res.blob());
}
