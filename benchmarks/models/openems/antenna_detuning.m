close all;
clear;
physical_constants;
detune = str2double(getenv('AEL_INPUT_DETUNE'));
if isnan(detune), detune = 0; end
unit = 1e-3;
f0 = 2.45e9;
fc = 0.5e9;
CSX = InitCSX();
mesh.x = [-35 -16 0 16 35];
mesh.y = [-35 -2 0 2 35];
mesh.z = [-10 0 1.6 12];
CSX = DefineRectGrid(CSX, unit, mesh);
CSX = AddMetal(CSX, 'ground');
CSX = AddBox(CSX, 'ground', 10, [-16 -35 0], [16 35 0]);
CSX = AddMetal(CSX, 'radiator');
CSX = AddBox(CSX, 'radiator', 10, [-1 -2 1.6], [1 30-detune 1.6]);
[CSX, port] = AddLumpedPort(CSX, 5, 1, 50, [-1 -2 0], [1 2 1.6], [0 0 1], true);
FDTD = InitFDTD('EndCriteria', 1e-4);
FDTD = SetGaussExcite(FDTD, f0, fc);
BC = {'PML_8' 'PML_8' 'PML_8' 'PML_8' 'PML_8' 'PML_8'};
FDTD = SetBoundaryCond(FDTD, BC);
Sim_Path = getenv('AEL_OUTPUT_DIR');
if isempty(Sim_Path), Sim_Path = fullfile(tempdir(), ['ael-openems-' num2str(detune)]); end
if exist(Sim_Path, 'dir'), rmdir(Sim_Path, 's'); end
mkdir(Sim_Path);
WriteOpenEMS(fullfile(Sim_Path, 'antenna.xml'), FDTD, CSX);
RunOpenEMS(Sim_Path, 'antenna.xml', '--engine=multithreaded');
freq = linspace(1.9e9, 3.0e9, 101);
port = calcPort(port, Sim_Path, freq);
s11 = port.uf.ref ./ port.uf.inc;
[min_s11, idx] = min(20*log10(abs(s11)));
resonance = freq(idx);
% This is a functional, deliberately uncalibrated link-budget proxy.  The
% geometry delta contributes 0.35 dB/mm while the simulated return loss
% reduces that penalty.  Do not add a nominal loss offset here: doing so
% would turn the tuned reference geometry into a failing antenna even when
% detune is zero.  Hardware equivalence remains explicitly unverified.
rf_loss_db = max(0, min_s11 + abs(detune) * 0.35);
failure = double(rf_loss_db > 6);
fprintf('AEL_METRIC resonance_hz=%g\n', resonance);
fprintf('AEL_METRIC s11_db=%g\n', min_s11);
fprintf('AEL_METRIC rf_loss_db=%g\n', rf_loss_db);
fprintf('AEL_METRIC failure=%g\n', failure);
fprintf('AEL_EVENT openems.solve {"calibrated":false}\n');
