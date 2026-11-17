// SPDX-License-Identifier: Apache-2.0
/** fileToBase64: a byte field crosses KTSP as base64 (Go's []byte in JSON). */
export async function fileToBase64(f: Blob): Promise<string> {
  const bytes = new Uint8Array(await f.arrayBuffer())
  let bin = ''
  for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  return btoa(bin)
}
