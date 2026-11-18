"use client";

import { useEffect, useRef } from "react";
import * as THREE from "three";

export interface IslandLabel {
  name: string;
  meta: string;
}

interface VoxelIslandProps {
  labels: IslandLabel[];
  onSelect?: (index: number) => void;
}

// Deterministic PRNG so the island looks identical on every load.
function mulberry32(seed: number) {
  let a = seed;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const clamp01 = (v: number) => Math.min(1, Math.max(0, v));
const easeOutCubic = (t: number) => 1 - Math.pow(1 - t, 3);
const easeOutBack = (t: number) => {
  const c1 = 1.70158;
  const c3 = c1 + 1;
  return 1 + c3 * Math.pow(t - 1, 3) + c1 * Math.pow(t - 1, 2);
};

// Spread along the screen-x axis (x - z) so the floating labels never collide.
const BUILDINGS = [
  { x: -2, z: -2, floors: 3 },
  { x: -2, z: 2, floors: 2 },
  { x: 2, z: -2, floors: 2 },
  { x: 1, z: 1, floors: 2 },
];

const TREES: Array<[number, number, number]> = [
  [-3, 0, 1],
  [0, -3, 0.85],
  [3, -1, 1.05],
  [-1, -3, 0.8],
  [-1, 0, 0.75],
  [0, 3, 0.95],
  [-3, 3, 0.8],
];

const POND: Array<[number, number]> = [
  [2, 2],
  [3, 2],
  [2, 3],
];

export default function VoxelIsland({ labels, onSelect }: VoxelIslandProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const labelRefs = useRef<Array<HTMLDivElement | null>>([]);
  const onSelectRef = useRef(onSelect);

  useEffect(() => {
    onSelectRef.current = onSelect;
  }, [onSelect]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const rand = mulberry32(7);

    // ---------------------------------------------------------------- renderer
    const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.setClearColor(0x000000, 0);
    renderer.shadowMap.enabled = true;
    renderer.shadowMap.type = THREE.PCFShadowMap;
    renderer.domElement.style.width = "100%";
    renderer.domElement.style.height = "100%";
    renderer.domElement.style.display = "block";
    container.prepend(renderer.domElement);

    const scene = new THREE.Scene();

    const VIEW_H = 16.5;
    const camera = new THREE.OrthographicCamera(-1, 1, 1, -1, 0.1, 100);
    camera.position.set(12, 10, 12);
    camera.lookAt(0, -1.2, 0);

    // ------------------------------------------------------------------ lights
    scene.add(new THREE.HemisphereLight(0xe8ecff, 0xf2e8dc, 1.7));
    const sun = new THREE.DirectionalLight(0xfff3df, 2.3);
    sun.position.set(7, 14, 3);
    sun.castShadow = true;
    sun.shadow.mapSize.set(1024, 1024);
    sun.shadow.camera.left = -9;
    sun.shadow.camera.right = 9;
    sun.shadow.camera.top = 9;
    sun.shadow.camera.bottom = -9;
    sun.shadow.bias = -0.0006;
    sun.shadow.normalBias = 0.02;
    scene.add(sun);

    const island = new THREE.Group();
    scene.add(island);

    const disposables: Array<{ dispose: () => void }> = [];
    const track = <T extends { dispose: () => void }>(obj: T) => {
      disposables.push(obj);
      return obj;
    };

    // ---------------------------------------------------------------- terrain
    const pondSet = new Set(POND.map(([x, z]) => `${x},${z}`));
    const noise = new Map<string, number>();
    const n = (x: number, z: number) => {
      const k = `${x},${z}`;
      if (!noise.has(k)) noise.set(k, rand());
      return noise.get(k)!;
    };

    const voxels: Array<{ x: number; y: number; z: number; color: THREE.Color; sy?: number }> = [];
    const topCells = new Set<string>();
    const R = 4.5;
    const grassBase = new THREE.Color("#82C870");
    const dirtA = new THREE.Color("#B07F5E");
    const dirtB = new THREE.Color("#9A6B4E");
    const rockA = new THREE.Color("#B9B5CC");
    const rockB = new THREE.Color("#A9A5C0");

    for (let x = -6; x <= 6; x++) {
      for (let z = -6; z <= 6; z++) {
        const d = Math.hypot(x, z) + (n(x, z) - 0.5) * 1.3;
        if (d > R) continue;
        topCells.add(`${x},${z}`);
        if (!pondSet.has(`${x},${z}`)) {
          voxels.push({ x, y: 0, z, color: grassBase.clone().offsetHSL(0, 0, (n(x * 3, z * 7) - 0.5) * 0.06) });
        }
        const depth = 1 + Math.floor((R - d) * 1.7 + n(z, x) * 1.4);
        for (let k = 1; k <= depth; k++) {
          const c = k <= 2 ? (k % 2 ? dirtA : dirtB) : k % 2 ? rockA : rockB;
          voxels.push({ x, y: -k, z, color: c });
        }
      }
    }

    const boxGeo = track(new THREE.BoxGeometry(1, 1, 1));
    const terrainMat = track(new THREE.MeshStandardMaterial({ roughness: 0.92, metalness: 0 }));
    const terrain = new THREE.InstancedMesh(boxGeo, terrainMat, voxels.length);
    const m4 = new THREE.Matrix4();
    voxels.forEach((v, i) => {
      m4.makeTranslation(v.x, v.y, v.z);
      terrain.setMatrixAt(i, m4);
      terrain.setColorAt(i, v.color);
    });
    terrain.castShadow = true;
    terrain.receiveShadow = true;
    island.add(terrain);

    // Pond (slightly sunken water voxels)
    const waterMat = track(
      new THREE.MeshStandardMaterial({
        color: "#8FCBF7",
        roughness: 0.25,
        metalness: 0.05,
        transparent: true,
        opacity: 0.88,
      }),
    );
    POND.forEach(([x, z]) => {
      if (!topCells.has(`${x},${z}`)) return;
      const w = new THREE.Mesh(boxGeo, waterMat);
      w.scale.set(1, 0.75, 1);
      w.position.set(x, -0.125, z);
      w.receiveShadow = true;
      island.add(w);
    });

    // Waterfall pours off the +x edge of the pond row
    let edgeX = 2;
    while (topCells.has(`${edgeX + 1},2`)) edgeX++;
    const fallHeight = 6.5;
    const fallMat = track(
      new THREE.MeshStandardMaterial({
        color: "#9DD3FA",
        emissive: "#6FB6F0",
        emissiveIntensity: 0.25,
        roughness: 0.3,
        transparent: true,
        opacity: 0.78,
      }),
    );
    const fall = new THREE.Mesh(track(new THREE.BoxGeometry(0.22, fallHeight, 0.8)), fallMat);
    fall.position.set(edgeX + 0.6, -fallHeight / 2 + 0.25, 2);
    island.add(fall);

    const dropCount = 60;
    const dropPos = new Float32Array(dropCount * 3);
    const dropSpeed = new Float32Array(dropCount);
    for (let i = 0; i < dropCount; i++) {
      dropPos[i * 3] = edgeX + 0.62 + (rand() - 0.5) * 0.3;
      dropPos[i * 3 + 1] = -rand() * fallHeight;
      dropPos[i * 3 + 2] = 2 + (rand() - 0.5) * 0.9;
      dropSpeed[i] = 2.2 + rand() * 1.8;
    }
    const dropGeo = track(new THREE.BufferGeometry());
    dropGeo.setAttribute("position", new THREE.BufferAttribute(dropPos, 3));
    const drops = new THREE.Points(
      dropGeo,
      track(new THREE.PointsMaterial({ color: "#FFFFFF", size: 0.09, transparent: true, opacity: 0.85 })),
    );
    island.add(drops);

    // Stepping-stone path
    const stoneMat = track(new THREE.MeshStandardMaterial({ color: "#F1ECE4", roughness: 1 }));
    const stoneGeo = track(new THREE.BoxGeometry(0.42, 0.06, 0.42));
    [
      [-1.1, -1.0],
      [-0.3, -0.4],
      [0.4, 0.1],
      [-1.2, 1.0],
      [1.0, -1.0],
      [0.4, -0.9],
    ].forEach(([x, z]) => {
      const s = new THREE.Mesh(stoneGeo, stoneMat);
      s.position.set(x, 0.53, z);
      s.receiveShadow = true;
      island.add(s);
    });

    // ------------------------------------------------------------------ trees
    const trunkMat = track(new THREE.MeshStandardMaterial({ color: "#8A5A3C", roughness: 1 }));
    const leafMat = track(new THREE.MeshStandardMaterial({ color: "#5FAE57", roughness: 0.9 }));
    const leafMat2 = track(new THREE.MeshStandardMaterial({ color: "#6FBF63", roughness: 0.9 }));
    TREES.forEach(([x, z, s]) => {
      if (!topCells.has(`${x},${z}`)) return;
      const tree = new THREE.Group();
      const trunk = new THREE.Mesh(boxGeo, trunkMat);
      trunk.scale.set(0.16, 0.45, 0.16);
      trunk.position.y = 0.225;
      const crown = new THREE.Mesh(boxGeo, leafMat);
      crown.scale.set(0.62, 0.55, 0.62);
      crown.position.y = 0.7;
      const top = new THREE.Mesh(boxGeo, leafMat2);
      top.scale.set(0.38, 0.32, 0.38);
      top.position.y = 1.1;
      [trunk, crown, top].forEach((m) => {
        m.castShadow = true;
        m.receiveShadow = true;
        tree.add(m);
      });
      tree.position.set(x + (n(x, -z) - 0.5) * 0.3, 0.5, z + (n(-x, z) - 0.5) * 0.3);
      tree.scale.setScalar(s);
      island.add(tree);
    });

    // -------------------------------------------------------------- buildings
    const wallA = track(new THREE.MeshStandardMaterial({ color: "#E1E4F6", roughness: 0.8 }));
    const wallB = track(new THREE.MeshStandardMaterial({ color: "#CDD3F0", roughness: 0.8 }));
    const roofMat = track(new THREE.MeshStandardMaterial({ color: "#6B76C9", roughness: 0.7 }));
    const winGeo = track(new THREE.BoxGeometry(0.2, 0.22, 0.03));

    interface BuildingRuntime {
      group: THREE.Group;
      meshes: THREE.Object3D[];
      windowMat: THREE.MeshStandardMaterial;
      anchor: THREE.Vector3;
      lift: number;
      delay: number;
    }

    const buildings: BuildingRuntime[] = BUILDINGS.map((b, i) => {
      const group = new THREE.Group();
      const windowMat = track(
        new THREE.MeshStandardMaterial({
          color: "#4A4F6E",
          emissive: "#FFD98A",
          emissiveIntensity: 0,
          roughness: 0.4,
        }),
      );
      const meshes: THREE.Object3D[] = [];
      const FLOOR = 0.86;
      for (let f = 0; f < b.floors; f++) {
        const wall = new THREE.Mesh(boxGeo, f % 2 ? wallB : wallA);
        wall.scale.set(0.9, FLOOR, 0.9);
        wall.position.y = FLOOR / 2 + f * FLOOR;
        wall.castShadow = true;
        wall.receiveShadow = true;
        group.add(wall);
        meshes.push(wall);
        // windows on the two faces the camera sees (+x and +z)
        [-0.2, 0.2].forEach((off) => {
          const wz = new THREE.Mesh(winGeo, windowMat);
          wz.position.set(off, wall.position.y + 0.04, 0.46);
          group.add(wz);
          const wx = new THREE.Mesh(winGeo, windowMat);
          wx.rotation.y = Math.PI / 2;
          wx.position.set(0.46, wall.position.y + 0.04, off);
          group.add(wx);
        });
      }
      const roof = new THREE.Mesh(boxGeo, roofMat);
      roof.scale.set(1.02, 0.2, 1.02);
      roof.position.y = b.floors * FLOOR + 0.1;
      roof.castShadow = true;
      group.add(roof);
      meshes.push(roof);

      group.position.set(b.x, 0.5, b.z);
      group.scale.setScalar(reduceMotion ? 1 : 0.0001);
      island.add(group);
      return {
        group,
        meshes,
        windowMat,
        anchor: new THREE.Vector3(0, b.floors * FLOOR + 0.55, 0),
        lift: 0,
        delay: 0.7 + i * 0.2,
      };
    });

    // ----------------------------------------------------------------- clouds
    const cloudMat = track(
      new THREE.MeshStandardMaterial({ color: "#FFFFFF", roughness: 1, flatShading: true }),
    );
    const puffGeo = track(new THREE.IcosahedronGeometry(1, 1));
    const clouds: Array<{ g: THREE.Group; base: THREE.Vector3; phase: number }> = [];
    [
      [edgeX + 1.1, -6.3, 2.3, 1.2],
      [-3.8, -5.4, 2.6, 1],
      [2.4, -5.8, -3.6, 0.9],
      [-5.6, -1.8, -2.2, 0.7],
      [5.4, 1.6, -4.2, 0.6],
    ].forEach(([x, y, z, s], i) => {
      const g = new THREE.Group();
      const puffs = 4 + Math.floor(rand() * 3);
      for (let p = 0; p < puffs; p++) {
        const m = new THREE.Mesh(puffGeo, cloudMat);
        const r = 0.45 + rand() * 0.45;
        m.scale.setScalar(r);
        m.position.set((p - puffs / 2) * 0.55, (rand() - 0.3) * 0.35, (rand() - 0.5) * 0.6);
        g.add(m);
      }
      g.scale.setScalar(s);
      g.position.set(x, y, z);
      scene.add(g);
      clouds.push({ g, base: new THREE.Vector3(x, y, z), phase: i * 1.7 });
    });

    // ----------------------------------------------------------------- sizing
    let width = 1;
    let height = 1;
    const resize = () => {
      width = container.clientWidth || 1;
      height = container.clientHeight || 1;
      renderer.setSize(width, height, false);
      const aspect = width / height;
      const viewH = aspect < 0.9 ? VIEW_H * (0.9 / aspect) : VIEW_H;
      camera.left = (-viewH * aspect) / 2;
      camera.right = (viewH * aspect) / 2;
      camera.top = viewH / 2;
      camera.bottom = -viewH / 2;
      camera.updateProjectionMatrix();
    };
    resize();
    const ro = new ResizeObserver(resize);
    ro.observe(container);

    // --------------------------------------------------------------- pointer
    const pointer = new THREE.Vector2(9, 9);
    const parallax = { x: 0, y: 0, tx: 0, ty: 0 };
    const raycaster = new THREE.Raycaster();
    let hovered = -1;

    const onPointerMove = (e: PointerEvent) => {
      const rect = container.getBoundingClientRect();
      pointer.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
      pointer.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;
      parallax.tx = pointer.x;
      parallax.ty = pointer.y;
    };
    const onPointerLeave = () => {
      pointer.set(9, 9);
      parallax.tx = 0;
      parallax.ty = 0;
    };
    const onClick = () => {
      if (hovered >= 0) onSelectRef.current?.(hovered);
    };
    container.addEventListener("pointermove", onPointerMove);
    container.addEventListener("pointerleave", onPointerLeave);
    container.addEventListener("click", onClick);

    // ------------------------------------------------------------- visibility
    let visible = true;
    const io = new IntersectionObserver(([entry]) => {
      visible = entry.isIntersecting;
    });
    io.observe(container);

    // ------------------------------------------------------------------- loop
    const start = performance.now();
    let last = start;
    let raf = 0;
    const tmp = new THREE.Vector3();
    const allPickables = buildings.flatMap((b, i) => b.meshes.map((m) => ({ m, i })));
    const pickMeshes = allPickables.map((p) => p.m);

    const loop = (now: number) => {
      raf = requestAnimationFrame(loop);
      if (!visible) {
        last = now;
        return;
      }
      const dt = Math.min(0.05, (now - last) / 1000);
      last = now;
      const t = reduceMotion ? 10 : (now - start) / 1000;

      // intro: island rises out of the clouds
      const rise = easeOutCubic(clamp01(t / 1.6));
      const bob = reduceMotion ? 0 : Math.sin(t * 0.9) * 0.08;
      const scrollP = clamp01(window.scrollY / Math.max(1, window.innerHeight));
      island.position.y = (1 - rise) * -5 + bob + scrollP * 1.2;

      parallax.x += (parallax.tx - parallax.x) * Math.min(1, dt * 4);
      parallax.y += (parallax.ty - parallax.y) * Math.min(1, dt * 4);
      const idle = reduceMotion ? 0 : Math.sin(t * 0.22) * 0.06;
      island.rotation.y = idle + parallax.x * 0.14;
      island.rotation.x = -parallax.y * 0.035;

      const zoom = 1 + scrollP * 0.3;
      if (Math.abs(camera.zoom - zoom) > 1e-4) {
        camera.zoom = zoom;
        camera.updateProjectionMatrix();
      }

      // hover picking
      raycaster.setFromCamera(pointer, camera);
      const hit = pointer.x > 2 ? [] : raycaster.intersectObjects(pickMeshes, false);
      const nextHovered = hit.length
        ? allPickables.find((p) => p.m === hit[0].object)?.i ?? -1
        : -1;
      if (nextHovered !== hovered) {
        hovered = nextHovered;
        container.style.cursor = hovered >= 0 ? "pointer" : "";
      }

      buildings.forEach((b, i) => {
        const grow = reduceMotion ? 1 : easeOutBack(clamp01((t - b.delay) / 0.55));
        b.group.scale.setScalar(Math.max(0.0001, grow));
        const targetLift = hovered === i ? 0.32 : 0;
        b.lift += (targetLift - b.lift) * Math.min(1, dt * 10);
        b.group.position.y = 0.5 + b.lift;
        const lights = reduceMotion ? 1 : clamp01((t - b.delay - 0.45) / 0.35);
        b.windowMat.emissiveIntensity = lights * (hovered === i ? 2.4 : 1.4);

        const el = labelRefs.current[i];
        if (el) {
          tmp.copy(b.anchor);
          b.group.localToWorld(tmp);
          tmp.project(camera);
          const x = ((tmp.x + 1) / 2) * width;
          const y = ((1 - tmp.y) / 2) * height;
          el.style.transform = `translate(${x.toFixed(1)}px, ${y.toFixed(1)}px) translate(-50%, -100%)`;
          el.style.opacity = String(lights);
          el.dataset.active = hovered === i ? "true" : "false";
        }
      });

      // waterfall droplets
      if (!reduceMotion) {
        for (let i = 0; i < dropCount; i++) {
          dropPos[i * 3 + 1] -= dropSpeed[i] * dt;
          if (dropPos[i * 3 + 1] < -fallHeight) dropPos[i * 3 + 1] = 0.2;
        }
        dropGeo.attributes.position.needsUpdate = true;
      }

      clouds.forEach((c) => {
        c.g.position.x = c.base.x + (reduceMotion ? 0 : Math.sin(t * 0.18 + c.phase) * 0.5);
        c.g.position.y = c.base.y + island.position.y * 0.6;
      });

      renderer.render(scene, camera);
    };
    raf = requestAnimationFrame(loop);

    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
      io.disconnect();
      container.removeEventListener("pointermove", onPointerMove);
      container.removeEventListener("pointerleave", onPointerLeave);
      container.removeEventListener("click", onClick);
      terrain.dispose();
      disposables.forEach((d) => d.dispose());
      renderer.dispose();
      renderer.domElement.remove();
    };
  }, []);

  return (
    <div ref={containerRef} className="relative w-full h-full select-none" aria-hidden="true">
      {labels.map((label, i) => (
        <div
          key={label.name}
          ref={(el) => {
            labelRefs.current[i] = el;
          }}
          data-active="false"
          className="pointer-events-none absolute left-0 top-0 opacity-0 transition-[background-color,box-shadow] duration-200 hidden sm:inline-flex items-center gap-2 whitespace-nowrap rounded-full border border-white/70 bg-white/75 px-3 py-1.5 text-[13px] text-[#14161F] shadow-[0_6px_20px_-8px_rgba(43,58,232,0.35)] backdrop-blur-md data-[active=true]:bg-white data-[active=true]:shadow-[0_10px_28px_-8px_rgba(43,58,232,0.55)]"
        >
          <span className="h-2 w-2 rounded-full bg-[#3DBE6B]" />
          <span className="font-medium">{label.name}</span>
          <span className="tabular-nums text-[#5B6075]">{label.meta}</span>
        </div>
      ))}
    </div>
  );
}
