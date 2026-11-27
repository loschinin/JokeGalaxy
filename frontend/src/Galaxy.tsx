import { useEffect, useRef, useState } from 'react';
import * as THREE from 'three';
import type { Joke } from './types';

interface GalaxyProps {
  jokes: Joke[];
  selectedId?: string;
  highlightedIds?: Set<string>;
  onSelect: (joke: Joke) => void;
  onHover?: (joke: Joke | null) => void;
}

export function Galaxy({ jokes, selectedId, highlightedIds, onSelect, onHover }: GalaxyProps) {
  const mountRef = useRef<HTMLDivElement>(null);
  const callbacksRef = useRef({ onSelect, onHover });
  const [isReady, setIsReady] = useState(false);
  callbacksRef.current = { onSelect, onHover };

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
    const visibleJokes = jokes.slice(0, 600);
    visibleJokes.forEach((joke, index) => {
      const isHighlighted = !highlightedIds?.size || highlightedIds.has(joke.id);
      const material = new THREE.MeshBasicMaterial({ color: joke.cluster.color, transparent: true, opacity: isHighlighted ? 0.96 : 0.19 });
      const point = new THREE.Mesh(pointGeometry, material);
      const fallbackAngle = index * 2.4;
      const x = Number.isFinite(joke.posX) ? joke.posX : Math.cos(fallbackAngle) * 0.5;
      const y = Number.isFinite(joke.posY) ? joke.posY : Math.sin(fallbackAngle) * 0.4;
      const z = Number.isFinite(joke.posZ) ? joke.posZ : Math.cos(fallbackAngle * 0.7) * 0.4;
      point.position.set(x * 4.4, y * 4.0, z * 4.4);
      point.scale.setScalar(joke.id === selectedId ? 1.55 : highlightedIds?.has(joke.id) ? 1.24 : 0.88 + (index % 4) * 0.07);
      point.userData.jokeId = joke.id;
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
      if (joke) callbacksRef.current.onSelect(joke);
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
      const selectedMesh = pointMeshes.find((mesh) => mesh.userData.jokeId === selectedId);
      halo.visible = Boolean(selectedMesh);
      if (selectedMesh) {
        halo.position.copy(selectedMesh.position);
        halo.rotation.set(camera.rotation.x, 0, 0);
        const pulse = 1 + Math.sin(performance.now() * 0.003) * 0.08;
        halo.scale.setScalar(pulse + 0.2);
      }
      renderer.render(scene, camera);
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
  }, [jokes, selectedId, highlightedIds]);

  return <div className={`galaxy-viewport ${isReady ? 'is-ready' : ''}`} ref={mountRef}>
    {!isReady && <div className="galaxy-fallback" aria-hidden="true"><span className="fallback-orbit orbit-one" /><span className="fallback-orbit orbit-two" /><span className="fallback-core" /></div>}
    <div className="galaxy-instructions"><span className="mouse-glyph">✣</span> Перетаскивайте · колесо — масштаб</div>
  </div>;
}
