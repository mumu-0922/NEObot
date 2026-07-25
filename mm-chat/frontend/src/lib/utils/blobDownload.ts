export function triggerBlobDownload(blob: Blob, filename: string): void {
  const objectUrl = URL.createObjectURL(blob);
  let anchor: HTMLAnchorElement | null = null;

  try {
    anchor = document.createElement("a");
    anchor.href = objectUrl;
    anchor.download = filename;
    anchor.hidden = true;
    document.body.appendChild(anchor);
    anchor.click();
  } finally {
    anchor?.remove();
    URL.revokeObjectURL(objectUrl);
  }
}
