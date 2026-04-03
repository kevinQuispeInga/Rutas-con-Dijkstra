console.log("app.js cargado");

// Esperar a que el DOM esté listo
window.addEventListener("load", () => {
  console.log("DOM listo, iniciando mapa...");

  // ---------- inicializar mapa ----------
  if (typeof L === "undefined") {
    console.error("Leaflet (L) no está definido. ¿Falló la carga del script de Leaflet?");
    return;
  }

  const map = L.map("map").setView([-12.07, -77.04], 12);

  L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", {
    maxZoom: 19,
    attribution: "&copy; OpenStreetMap contributors",
  }).addTo(map);

  let userMarker = null;
  let routePolyline = null;
  let recyclingMarkers = [];
  let userLat = null;
  let userLon = null;

  const btnUbicacion = document.getElementById("btnUbicacion");
  const estado = document.getElementById("estado");
  const selectRecycling = document.getElementById("recyclingSelect");
  const rutaResumen = document.getElementById("rutaResumen");
  const callesList = document.getElementById("callesList");

  function setEstado(msg) {
    estado.textContent = msg || "";
  }

  function limpiarRuta() {
    if (routePolyline) {
      map.removeLayer(routePolyline);
      routePolyline = null;
    }
    callesList.innerHTML = "";
    rutaResumen.textContent =
      "Selecciona un punto de reciclaje para ver la ruta.";
  }

  function limpiarRecyclingMarkers() {
    recyclingMarkers.forEach((m) => map.removeLayer(m));
    recyclingMarkers = [];
  }

  // ---------- geolocalización ----------
  btnUbicacion.addEventListener("click", () => {
    console.log("Click en 'Usar mi ubicación'");
    if (!navigator.geolocation) {
      setEstado("Geolocalización no soportada por tu navegador.");
      return;
    }

    setEstado("Obteniendo ubicación...");
    btnUbicacion.disabled = true;

    navigator.geolocation.getCurrentPosition(
      (pos) => {
        userLat = pos.coords.latitude;
        userLon = pos.coords.longitude;
        console.log("Ubicación obtenida:", userLat, userLon);
        setEstado(
          `Ubicación: lat=${userLat.toFixed(5)}, lon=${userLon.toFixed(5)}`
        );
        btnUbicacion.disabled = false;

        if (userMarker) {
          map.removeLayer(userMarker);
        }
        userMarker = L.marker([userLat, userLon]).addTo(map);
        userMarker.bindPopup("Tú estás aquí").openPopup();
        map.setView([userLat, userLon], 15);

        limpiarRuta();
        cargarPuntosReciclaje();
      },
      (err) => {
        console.error("Error geolocalización:", err);
        setEstado("No se pudo obtener tu ubicación.");
        btnUbicacion.disabled = false;
      }
    );
  });

  // ---------- cargar puntos de reciclaje ----------
  async function cargarPuntosReciclaje() {
    if (userLat == null || userLon == null) return;

    try {
      setEstado("Cargando puntos de reciclaje...");
      limpiarRecyclingMarkers();
      selectRecycling.innerHTML = "";
      selectRecycling.disabled = true;
      limpiarRuta();

      const url = `/api/puntos_reciclaje?lat=${encodeURIComponent(
        userLat
      )}&lon=${encodeURIComponent(userLon)}`;
      console.log("GET", url);
      const resp = await fetch(url);
      if (!resp.ok) {
        throw new Error("Error HTTP " + resp.status);
      }

      const data = await resp.json();

      if (!Array.isArray(data) || data.length === 0) {
        selectRecycling.innerHTML =
          '<option value="">No se encontraron puntos de reciclaje</option>';
        setEstado("No se encontraron puntos de reciclaje.");
        return;
      }

      const options = [];
      for (const item of data) {
        const name =
          item.name && item.name.trim().length > 0
            ? item.name
            : "Punto sin nombre";

        const label = `${name} (${item.distance_km.toFixed(2)} km)`;
        const opt = document.createElement("option");
        opt.value = String(item.id);
        opt.textContent = label;
        options.push(opt);

        const marker = L.marker([item.lat, item.lon]).addTo(map);
        marker.bindPopup(label);
        recyclingMarkers.push(marker);
      }

      selectRecycling.appendChild(
        new Option("Selecciona un punto...", "", true, false)
      );
      options.forEach((o) => selectRecycling.appendChild(o));
      selectRecycling.disabled = false;

      setEstado(`Se encontraron ${data.length} puntos de reciclaje cercanos.`);
    } catch (err) {
      console.error("Error cargar puntos:", err);
      setEstado("Error obteniendo puntos de reciclaje.");
      selectRecycling.innerHTML =
        '<option value="">Error cargando puntos</option>';
      selectRecycling.disabled = true;
    }
  }

  // ---------- trazar ruta cuando el usuario elige un punto ----------
  selectRecycling.addEventListener("change", () => {
    const idStr = selectRecycling.value;
    if (!idStr) {
      limpiarRuta();
      return;
    }
    if (userLat == null || userLon == null) {
      setEstado("Primero usa tu ubicación.");
      return;
    }
    trazarRuta(parseInt(idStr, 10));
  });

  async function trazarRuta(recyclingId) {
    try {
      setEstado("Calculando ruta...");
      limpiarRuta();

      const url = `/api/ruta_reciclaje?lat=${encodeURIComponent(
        userLat
      )}&lon=${encodeURIComponent(
        userLon
      )}&recycling_id=${encodeURIComponent(recyclingId)}`;
      console.log("GET", url);

      const resp = await fetch(url);
      if (!resp.ok) {
        const txt = await resp.text();
        console.error("Error backend:", resp.status, txt);
        setEstado("No se pudo trazar la ruta hasta ese punto.");
        return;
      }

      const data = await resp.json();

      if (!Array.isArray(data.coords) || data.coords.length === 0) {
        setEstado("Respuesta sin coordenadas de ruta.");
        return;
      }

      const latlngs = data.coords.map((c) => [c[0], c[1]]);
      routePolyline = L.polyline(latlngs, { color: "red", weight: 4 }).addTo(
        map
      );
      map.fitBounds(routePolyline.getBounds(), { padding: [20, 20] });

      rutaResumen.textContent = `Distancia aproximada: ${data.distance_km.toFixed(
        2
      )} km`;

      callesList.innerHTML = "";
      if (Array.isArray(data.streets) && data.streets.length > 0) {
        data.streets.forEach((s) => {
          const li = document.createElement("li");
          li.textContent = s;
          callesList.appendChild(li);
        });
      } else {
        const li = document.createElement("li");
        li.textContent = "No se pudo obtener la lista de calles.";
        callesList.appendChild(li);
      }

      setEstado("");
    } catch (err) {
      console.error("Error calculando ruta:", err);
      setEstado("Error calculando ruta.");
    }
  }
});
