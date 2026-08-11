import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import pngToIco from "png-to-ico";
import sharp from "sharp";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const root = resolve(scriptDir, "../..");
const source = join(root, "build/appicon.svg");
const appIcon = join(root, "build/appicon.png");
const windowsIcon = join(root, "build/windows/icon.ico");
const linuxIcon = join(root, "build/linux/icon.png");
const browserIcon = join(root, "frontend/public/molex-mark.png");
const temporary = await mkdtemp(join(tmpdir(), "molex-icons-"));

try {
  const svg = await readFile(source);
  await Promise.all([
    mkdir(dirname(windowsIcon), { recursive: true }),
    mkdir(dirname(linuxIcon), { recursive: true }),
  ]);
  await sharp(svg).resize(1024, 1024).png().toFile(appIcon);
  await sharp(svg).resize(512, 512).png().toFile(linuxIcon);
  await sharp(svg).resize(256, 256).png().toFile(browserIcon);

  const sizes = [16, 24, 32, 48, 64, 128, 256];
  const files = await Promise.all(
    sizes.map(async (size) => {
      const output = join(temporary, `molex-${size}.png`);
      await sharp(svg).resize(size, size).png().toFile(output);
      return output;
    }),
  );
  await writeFile(windowsIcon, await pngToIco(files));
  console.log("Generated MoleX PNG and Windows icon assets");
} finally {
  await rm(temporary, { recursive: true, force: true });
}
