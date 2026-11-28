import { useEffect, useRef, useState } from 'react';
import { ArrowRight, LoaderCircle, Maximize2, Minimize2, Search, X } from 'lucide-react';
import * as THREE from 'three';
import type { Joke } from './types';

interface GalaxyProps {
  jokes: Joke[];
  selectedId?: string;
  highlightedIds?: Set<string>;
  searchText: string;
  submittedQuery: string;
  isSearching: boolean;
  onSearchTextChange: (value: string) => void;
  onSearch: (query: string) => void;
  onSelect: (joke: Joke | null) => void;
  onHover?: (joke: Joke | null) => void;
}

type GalaxyPosition = [number, number, number];

const MAP_RADIUS = 3.1;
const MAX_MAP_COORDINATE = 3.45;

function hashToUnit(value: string, salt: number): number {
  let hash = (2166136261 ^ salt) >>> 0;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619) >>> 0;
  }
  return hash / 0xffffffff;
}

function fallbackPosition(id: string): GalaxyPosition {
  const latitude = Math.asin(2 * hashToUnit(id, 0) - 1);
  const longitude = 2 * Math.PI * hashToUnit(id, 1);
  const radius = MAP_RADIUS * 0.86;
  const ring = Math.cos(latitude) * radius;
  return [ring * Math.cos(longitude), Math.sin(latitude) * radius, ring * Math.sin(longitude)];
}

function quantile(sorted: number[], fraction: number): number {
  const rank = (sorted.length - 1) * fraction;
  const lower = Math.floor(rank);
  const upper = Math.ceil(rank);
  const weight = rank - lower;
  return sorted[lower] * (1 - weight) + sorted[upper] * weight;
}

function projectGalaxyPositions(jokes: Joke[]): GalaxyPosition[] {
  if (jokes.length === 0) return [];
  if (jokes.length === 1) return [[0, 0, 0]];

  const coordinates = jokes.map((joke, index) => {
    const angle = index * 2.4;
    return [
      Number.isFinite(joke.posX) ? joke.posX : Math.cos(angle) * 0.5,
      Number.isFinite(joke.posY) ? joke.posY : Math.sin(angle) * 0.4,
      Number.isFinite(joke.posZ) ? joke.posZ : Math.cos(angle * 0.7) * 0.4,
    ] as GalaxyPosition;
  });
  const axes = [0, 1, 2].map((axis) => coordinates.map((point) => point[axis]).sort((a, b) => a - b));
  const lower = axes.map((axis) => quantile(axis, 0.02));
  const upper = axes.map((axis) => quantile(axis, 0.98));
  const center = lower.map((value, axis) => (value + upper[axis]) / 2);
  const span = Math.max(...lower.map((value, axis) => upper[axis] - value));

  if (!Number.isFinite(span) || span < 1e-5) {
    return jokes.map((joke, index) => fallbackPosition(joke.id || String(index)));
  }

  const scale = (MAP_RADIUS * 2) / span;
  return coordinates.map((point) => point.map((value, axis) => (
    Math.max(-MAX_MAP_COORDINATE, Math.min(MAX_MAP_COORDINATE, (value - center[axis]) * scale))
  )) as GalaxyPosition);
}

export function Galaxy({
  jokes, selectedId, highlightedIds, searchText, submittedQuery, isSearching,
  onSearchTextChange, onSearch, onSelect, onHover,
}: GalaxyProps) {
  const mountRef = useRef<HTMLDivElement>(null);
  const selectedInfoRef = useRef<HTMLDivElement>(null);
  const selectedIdRef = useRef(selectedId);
  const highlightedIdsRef = useRef(highlightedIds);
  const callbacksRef = useRef({ onSelect, onHover });
  const selectedJoke = selectedId ? jokes.find((joke) => joke.id === selectedId) : undefined;
  const visibleSearchCount = highlightedIds
    ? jokes.slice(0, 600).filter((joke) => highlightedIds.has(joke.id)).length
    : 0;
  const [isReady, setIsReady] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [fullscreenSupported, setFullscreenSupported] = useState(false);
  selectedIdRef.current = selectedId;
  highlightedIdsRef.current = highlightedIds;
  callbacksRef.current = { onSelect, onHover };

  useEffect(() => {
    const mount = mountRef.current;
    if (!mount) return;
    setFullscreenSupported(Boolean(document.fullscreenEnabled && typeof mount.requestFullscreen === 'function'));
    const syncFullscreenState = () => setIsFullscreen(document.fullscreenElement === mount);
    document.addEventListener('fullscreenchange', syncFullscreenState);
    syncFullscreenState();
    return () => document.removeEventListener('fullscreenchange', syncFullscreenState);
  }, []);

  const toggleFullscreen = async () => {
    const mount = mountRef.current;
    if (!mount || !fullscreenSupported) return;
    try {
      if (document.fullscreenElement === mount) await document.exitFullscreen();
      else await mount.requestFullscreen();
    } catch {
      // Keep the inline map usable if the browser or embedding context denies fullscreen.
    }
  };

  useEffect(() => {
    const mount = mountRef.current;
    if (!mount) return;
    let renderer: THREE.WebGLRenderer;
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, powerPreference: 'low-power' });
    } catch {
      mount.dataset.webglUnavailable = 'true';
      return;
    }
    const scene = new THREE.Scene();
    const camera = new THREE.PerspectiveCamera(43, 1, 0.1, 100);
    camera.position.set(0, 0, 9.8);
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.7));
    renderer.setClearColor(0x000000, 0);
    renderer.domElement.className = 'galaxy-canvas';
    renderer.domElement.setAttribute('aria-label', 'Интерактивная трёхмерная карта шуток. Перетаскивайте, чтобы вращать; нажмите на точку, чтобы открыть шутку.');
    renderer.domElement.setAttribute('role', 'img');
    mount.appendChild(renderer.domElement);

    const cloud = new THREE.Group();
    scene.add(cloud);
    const pointGeometry = new THREE.SphereGeometry(0.065, 12, 10);
    const pointMeshes: THREE.Mesh[] = [];
    const jokeForMesh = new Map<THREE.Object3D, Joke>();
    const layoutPositions = projectGalaxyPositions(jokes);
    const visibleJokes = jokes.slice(0, 600);
    visibleJokes.forEach((joke, index) => {
      const currentHighlights = highlightedIdsRef.current;
      const isHighlighted = !currentHighlights || currentHighlights.has(joke.id);
      const material = new THREE.MeshBasicMaterial({ color: joke.cluster.color, transparent: true, opacity: isHighlighted ? 0.96 : 0.19 });
      const point = new THREE.Mesh(pointGeometry, material);
      const [x, y, z] = layoutPositions[index];
      const baseScale = 0.88 + (index % 4) * 0.07;
      const pointScale = currentHighlights?.has(joke.id) ? 1.24 : baseScale;
      point.position.set(x, y, z);
      point.scale.setScalar(joke.id === selectedIdRef.current ? 1.55 : pointScale);
      point.userData.jokeId = joke.id;
      point.userData.defaultScale = baseScale;
      cloud.add(point);
      pointMeshes.push(point);
      jokeForMesh.set(point, joke);
    });

    const starCount = 500;
    const starPositions = new Float32Array(starCount * 3);
    let seed = 2048;
    const rand = () => { seed = (seed * 16807) % 2147483647; return (seed - 1) / 2147483646; };
    for (let i = 0; i < starCount; i += 1) {
      starPositions[i * 3] = (rand() - 0.5) * 19;
      starPositions[i * 3 + 1] = (rand() - 0.5) * 14;
      starPositions[i * 3 + 2] = (rand() - 0.5) * 12 - 2;
    }
    const starGeometry = new THREE.BufferGeometry();
    starGeometry.setAttribute('position', new THREE.BufferAttribute(starPositions, 3));
    const starMaterial = new THREE.PointsMaterial({ color: 0xabb9e9, size: 0.018, transparent: true, opacity: 0.48, sizeAttenuation: true });
    const stars = new THREE.Points(starGeometry, starMaterial);
    scene.add(stars);

    const orbitMaterial = new THREE.MeshBasicMaterial({ color: 0x545c91, transparent: true, opacity: 0.19, side: THREE.DoubleSide });
    const orbitA = new THREE.Mesh(new THREE.TorusGeometry(2.75, 0.006, 4, 100), orbitMaterial);
    orbitA.rotation.set(0.86, 0.25, -0.25);
    cloud.add(orbitA);
    const orbitB = new THREE.Mesh(new THREE.TorusGeometry(1.72, 0.004, 4, 100), new THREE.MeshBasicMaterial({ color: 0x6c568f, transparent: true, opacity: 0.14, side: THREE.DoubleSide }));
    orbitB.rotation.set(1.1, -0.42, 0.24);
    cloud.add(orbitB);

    const halo = new THREE.Mesh(new THREE.RingGeometry(0.1, 0.15, 32), new THREE.MeshBasicMaterial({ color: 0xffffff, transparent: true, opacity: 0.83, side: THREE.DoubleSide }));
    halo.visible = false;
    cloud.add(halo);
    const projectedSelectedPosition = new THREE.Vector3();
    let activeSelectedId = selectedIdRef.current;
    let activeHighlightedIds = highlightedIdsRef.current;
    let lastSelectedAnchor = { x: 0, y: 0 };
    const raycaster = new THREE.Raycaster();
    const pointer = new THREE.Vector2();
    let width = 1;
    let height = 1;
    let animationFrame = 0;
    let isDragging = false;
    let pointerStart = { x: 0, y: 0 };
    let pointerLast = { x: 0, y: 0 };
    let didDrag = false;
    let hoveredId: string | null = null;
    const resize = () => {
      const bounds = mount.getBoundingClientRect();
      width = Math.max(1, bounds.width);
      height = Math.max(1, bounds.height);
      renderer.setSize(width, height, false);
      camera.aspect = width / height;
      camera.updateProjectionMatrix();
    };
    resize();
    lastSelectedAnchor = { x: width / 2, y: height / 2 };
    const resizeObserver = new ResizeObserver(resize);
    resizeObserver.observe(mount);

    const setFromEvent = (event: PointerEvent) => {
      const bounds = renderer.domElement.getBoundingClientRect();
      pointer.x = ((event.clientX - bounds.left) / bounds.width) * 2 - 1;
      pointer.y = -((event.clientY - bounds.top) / bounds.height) * 2 + 1;
      raycaster.setFromCamera(pointer, camera);
    };
    const handlePointerDown = (event: PointerEvent) => {
      isDragging = true;
      didDrag = false;
      pointerStart = { x: event.clientX, y: event.clientY };
      pointerLast = pointerStart;
      renderer.domElement.setPointerCapture?.(event.pointerId);
    };
    const handlePointerMove = (event: PointerEvent) => {
      if (isDragging) {
        const dx = event.clientX - pointerLast.x;
        const dy = event.clientY - pointerLast.y;
        if (Math.abs(event.clientX - pointerStart.x) + Math.abs(event.clientY - pointerStart.y) > 5) didDrag = true;
        cloud.rotation.y += dx * 0.005;
        cloud.rotation.x += dy * 0.0035;
        cloud.rotation.x = Math.max(-0.9, Math.min(0.9, cloud.rotation.x));
        pointerLast = { x: event.clientX, y: event.clientY };
        return;
      }
      setFromEvent(event);
      const hit = raycaster.intersectObjects(pointMeshes, false)[0];
      const joke = hit ? jokeForMesh.get(hit.object) ?? null : null;
      const id = joke?.id ?? null;
      if (id !== hoveredId) {
        hoveredId = id;
        renderer.domElement.style.cursor = joke ? 'pointer' : 'grab';
        callbacksRef.current.onHover?.(joke);
      }
    };
    const handlePointerUp = (event: PointerEvent) => {
      if (!isDragging) return;
      isDragging = false;
      renderer.domElement.releasePointerCapture?.(event.pointerId);
      if (didDrag) return;
      setFromEvent(event);
      const hit = raycaster.intersectObjects(pointMeshes, false)[0];
      const joke = hit ? jokeForMesh.get(hit.object) : undefined;
      callbacksRef.current.onSelect(joke ?? null);
    };
    const handleWheel = (event: WheelEvent) => {
      event.preventDefault();
      camera.position.z = THREE.MathUtils.clamp(camera.position.z + event.deltaY * 0.008, 5.5, 15);
    };
    const handleLeave = () => {
      isDragging = false;
      hoveredId = null;
      callbacksRef.current.onHover?.(null);
      renderer.domElement.style.cursor = 'grab';
    };
    renderer.domElement.addEventListener('pointerdown', handlePointerDown);
    renderer.domElement.addEventListener('pointermove', handlePointerMove);
    renderer.domElement.addEventListener('pointerup', handlePointerUp);
    renderer.domElement.addEventListener('pointercancel', handleLeave);
    renderer.domElement.addEventListener('pointerleave', handleLeave);
    renderer.domElement.addEventListener('wheel', handleWheel, { passive: false });

    const animate = () => {
      animationFrame = window.requestAnimationFrame(animate);
      cloud.rotation.y += isDragging ? 0 : 0.00032;
      stars.rotation.y -= 0.000035;

      const currentSelectedId = selectedIdRef.current;
      const currentHighlightedIds = highlightedIdsRef.current;
      if (currentSelectedId !== activeSelectedId) {
        const previousMesh = pointMeshes.find((mesh) => mesh.userData.jokeId === activeSelectedId);
        if (previousMesh) {
          const wasSearchMatch = currentHighlightedIds?.has(previousMesh.userData.jokeId);
          previousMesh.scale.setScalar(wasSearchMatch ? 1.24 : previousMesh.userData.defaultScale ?? 1);
        }
        const nextMesh = pointMeshes.find((mesh) => mesh.userData.jokeId === currentSelectedId);
        nextMesh?.scale.setScalar(1.55);
        activeSelectedId = currentSelectedId;
      }
      if (currentHighlightedIds !== activeHighlightedIds) {
        pointMeshes.forEach((mesh) => {
          const jokeId = mesh.userData.jokeId;
          const isHighlighted = !currentHighlightedIds || currentHighlightedIds.has(jokeId);
          const material = mesh.material as THREE.MeshBasicMaterial;
          material.opacity = isHighlighted ? 0.96 : 0.19;
          mesh.scale.setScalar(jokeId === currentSelectedId ? 1.55 : currentHighlightedIds?.has(jokeId) ? 1.24 : mesh.userData.defaultScale ?? 1);
        });
        activeHighlightedIds = currentHighlightedIds;
      }
      const selectedMesh = pointMeshes.find((mesh) => mesh.userData.jokeId === currentSelectedId);
      halo.visible = Boolean(selectedMesh);
      if (selectedMesh) {
        halo.position.copy(selectedMesh.position);
        halo.rotation.set(camera.rotation.x, 0, 0);
        const pulse = 1 + Math.sin(performance.now() * 0.003) * 0.08;
        halo.scale.setScalar(pulse + 0.2);
      }
      renderer.render(scene, camera);

      const selectedInfo = selectedInfoRef.current;
      if (selectedInfo) {
        if (selectedMesh) {
          selectedMesh.getWorldPosition(projectedSelectedPosition);
          projectedSelectedPosition.project(camera);
          if (projectedSelectedPosition.z >= -1 && projectedSelectedPosition.z <= 1) {
            lastSelectedAnchor = {
              x: Math.min(width - 8, Math.max(8, (projectedSelectedPosition.x + 1) * 0.5 * width)),
              y: Math.min(height - 8, Math.max(8, (1 - projectedSelectedPosition.y) * 0.5 * height)),
            };
          }

          const infoWidth = selectedInfo.offsetWidth;
          const infoHeight = selectedInfo.offsetHeight;
          const gap = 14;
          const margin = 12;
          const safeTop = document.fullscreenElement === mount ? (width <= 640 ? 154 : 106) : 88;
          const safeBottom = 44;
          const canPlaceRight = lastSelectedAnchor.x + gap + infoWidth <= width - margin;
          const canPlaceLeft = lastSelectedAnchor.x - gap - infoWidth >= margin;
          let side: 'right' | 'left' | 'top' | 'bottom';
          let left: number;
          let top: number;

          if (canPlaceRight) {
            side = 'right';
            left = lastSelectedAnchor.x + gap;
            top = lastSelectedAnchor.y - infoHeight / 2;
          } else if (canPlaceLeft) {
            side = 'left';
            left = lastSelectedAnchor.x - infoWidth - gap;
            top = lastSelectedAnchor.y - infoHeight / 2;
          } else {
            left = Math.min(width - infoWidth - margin, Math.max(margin, lastSelectedAnchor.x - infoWidth / 2));
            const canPlaceBelow = lastSelectedAnchor.y + gap + infoHeight <= height - safeBottom;
            side = canPlaceBelow ? 'bottom' : 'top';
            top = canPlaceBelow ? lastSelectedAnchor.y + gap : lastSelectedAnchor.y - infoHeight - gap;
          }

          const maxLeft = Math.max(margin, width - infoWidth - margin);
          const maxTop = Math.max(safeTop, height - infoHeight - safeBottom);
          selectedInfo.dataset.side = side;
          selectedInfo.style.transform = `translate3d(${Math.min(maxLeft, Math.max(margin, left))}px, ${Math.min(maxTop, Math.max(safeTop, top))}px, 0)`;
          selectedInfo.style.visibility = 'visible';
        } else {
          selectedInfo.style.visibility = 'hidden';
        }
      }
    };
    animate();
    setIsReady(true);

    return () => {
      window.cancelAnimationFrame(animationFrame);
      resizeObserver.disconnect();
      renderer.domElement.removeEventListener('pointerdown', handlePointerDown);
      renderer.domElement.removeEventListener('pointermove', handlePointerMove);
      renderer.domElement.removeEventListener('pointerup', handlePointerUp);
      renderer.domElement.removeEventListener('pointercancel', handleLeave);
      renderer.domElement.removeEventListener('pointerleave', handleLeave);
      renderer.domElement.removeEventListener('wheel', handleWheel);
      pointGeometry.dispose();
      starGeometry.dispose();
      starMaterial.dispose();
      orbitMaterial.dispose();
      cloud.traverse((object) => {
        if (object instanceof THREE.Mesh && object.geometry !== pointGeometry) {
          object.geometry.dispose();
          const material = object.material;
          if (Array.isArray(material)) material.forEach((entry) => entry.dispose());
          else material.dispose();
        }
      });
      pointMeshes.forEach((mesh) => (mesh.material as THREE.Material).dispose());
      renderer.dispose();
      renderer.domElement.remove();
      setIsReady(false);
    };
  }, [jokes]);

  return <div className={`galaxy-viewport ${isReady ? 'is-ready' : ''}`} ref={mountRef}>
    {!isReady && <div className="galaxy-fallback" aria-hidden="true"><span className="fallback-orbit orbit-one" /><span className="fallback-orbit orbit-two" /><span className="fallback-core" /></div>}
    {selectedJoke && <div
      className="galaxy-selected-info"
      ref={selectedInfoRef}
      role="status"
      aria-live="polite"
      aria-atomic="true"
      style={{ visibility: 'hidden' }}
    >
      <div className="galaxy-selected-cluster">
        <span className="galaxy-selected-cluster-dot" style={{ backgroundColor: selectedJoke.cluster.color }} aria-hidden="true" />
        <span className="galaxy-selected-cluster-label">КЛАСТЕР</span>
        <span className="galaxy-selected-cluster-name">{selectedJoke.cluster.name}</span>
      </div>
      <p className="galaxy-selected-text">{selectedJoke.text}</p>
    </div>}
    {fullscreenSupported && <button
      type="button"
      className="galaxy-fullscreen-toggle"
      onClick={() => void toggleFullscreen()}
      aria-label={isFullscreen ? 'Выйти из полноэкранного режима' : 'Развернуть карту на весь экран'}
      aria-pressed={isFullscreen}
      title={isFullscreen ? 'Выйти из полноэкранного режима' : 'Развернуть карту на весь экран'}
    >
      {isFullscreen ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
      <span>{isFullscreen ? 'Выйти из полного экрана' : 'На весь экран'}</span>
    </button>}
    {isFullscreen && <form
      className="galaxy-fullscreen-search"
      onSubmit={(event) => { event.preventDefault(); onSearch(searchText); }}
    >
      <Search size={16} className="galaxy-fullscreen-search-icon" aria-hidden="true" />
      <input
        type="text"
        value={searchText}
        onChange={(event) => onSearchTextChange(event.target.value)}
        placeholder="Найти шутку на карте"
        aria-label="Поиск шуток на полноэкранной карте"
        autoComplete="off"
      />
      {(searchText || submittedQuery) && <button
        type="button"
        className="galaxy-fullscreen-search-clear"
        onClick={() => { onSearchTextChange(''); onSearch(''); }}
        aria-label="Очистить поиск и подсветку"
        title="Очистить поиск"
      ><X size={14} /></button>}
      <button type="submit" className="galaxy-fullscreen-search-submit" aria-label="Искать на карте" title="Искать на карте">
        {isSearching ? <LoaderCircle size={16} className="spin" /> : <ArrowRight size={16} />}
      </button>
    </form>}
    {isFullscreen && (submittedQuery || isSearching) && <div className="galaxy-fullscreen-search-status" role="status" aria-live="polite">
      {isSearching
        ? <><LoaderCircle size={12} className="spin" /> Ищем «{searchText.trim() || submittedQuery}»…</>
        : visibleSearchCount
          ? <>Подсвечено {visibleSearchCount} точек · «{submittedQuery}»</>
          : <>На карте нет совпадений · «{submittedQuery}»</>}
    </div>}
    <div className="galaxy-instructions"><span className="mouse-glyph">✣</span> Перетаскивайте · колесо — масштаб</div>
  </div>;
}
