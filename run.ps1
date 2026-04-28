# MinerU File Organization
# 1. Copy full.md -> output/temp/subject_partN.md (originals stay)
# 2. Merge same-subject parts -> output/subject.md
# 3. Collect all images -> output/images/

$root = "e:\LingNc\MinerU"
$out  = "$root\output"
$tmp  = "$out\temp"
$imgs = "$out\images"

# Clean & create
if (Test-Path $out) { Remove-Item $out -Recurse -Force }
New-Item -ItemType Directory -Force -Path $tmp  | Out-Null
New-Item -ItemType Directory -Force -Path $imgs | Out-Null

$groups = @{}

Write-Output "=========================================="
Write-Output "  MinerU File Organizer"
Write-Output "=========================================="
Write-Output ""

# ---- Step 1: Copy md + images ----
Write-Output "[1/3] Copying full.md -> output/temp/ and images -> output/images/ ..."

$dirs = Get-ChildItem $root -Directory | Where-Object { $_.Name -match '_part\d+' }
$m = 0; $i = 0

foreach ($d in $dirs) {
    $n = $d.Name
    if ($n -match '^(.+?)_part(\d+)') {
        $sub = $Matches[1]
        $pn  = $Matches[2]
    } else { continue }

    Write-Output "  $sub (part$pn)"

    if (-not $groups.ContainsKey($sub)) { $groups[$sub] = @() }
    $groups[$sub] += $pn

    # copy full.md -> output/temp/subject_partN.md
    $src = "$($d.FullName)\full.md"
    $dst = "$tmp\$($sub)_$pn.md"
    if (Test-Path $src) {
        Copy-Item $src $dst -Force
        $m++
    } else { Write-Output "    WARNING: no full.md" }

    # copy images -> output/images/
    $isrc = "$($d.FullName)\images"
    if (Test-Path $isrc) {
        Get-ChildItem $isrc -File | ForEach-Object {
            $idst = "$imgs\$($_.Name)"
            if (-not (Test-Path $idst)) {
                Copy-Item $_.FullName $idst -Force
                $i++
            }
        }
    } else { Write-Output "    WARNING: no images/" }
}

Write-Output "  MD: $m, Images: $i"
Write-Output ""

# ---- Step 2: Merge parts ----
Write-Output "[2/3] Merging parts -> output/ ..."

$mc = 0
foreach ($sub in $groups.Keys) {
    $ps = $groups[$sub] | Sort-Object { [int]$_ }

    if ($ps.Count -eq 1) {
        # rename & move single part to output root
        $sf = "$tmp\$($sub)_$($ps[0]).md"
        $mf = "$out\$sub.md"
        if (Test-Path $sf) {
            Move-Item $sf $mf -Force
            Write-Output "  $sub`_$($ps[0]).md -> $sub.md"
        }
        continue
    }

    Write-Output "  Merging: $sub (parts $ps)"

    $parts = @()
    foreach ($p in $ps) {
        $pf = "$tmp\$($sub)_$p.md"
        if (Test-Path $pf) {
            $txt = Get-Content $pf -Raw -Encoding UTF8
            if ($txt) { $parts += $txt.TrimEnd() }
        }
    }

    $mf = "$out\$sub.md"
    $parts -join "`n`n---`n`n" | Out-File $mf -Encoding UTF8 -NoNewline
    $mc++
}

Write-Output "  Merged: $mc"
Write-Output ""

# ---- Step 3: Summary ----
Write-Output "[3/3] Summary"
Write-Output "=========================================="

Write-Output ""
Write-Output "  output/ (merged):"
Get-ChildItem $out -Filter "*.md" | ForEach-Object {
    $kb = [math]::Round($_.Length / 1KB, 1)
    Write-Output "    $($_.Name)  ($kb KB)"
}

Write-Output ""
Write-Output "  output/temp/ (parts):"
Get-ChildItem $tmp -Filter "*.md" | ForEach-Object {
    $kb = [math]::Round($_.Length / 1KB, 1)
    Write-Output "    $($_.Name)  ($kb KB)"
}

$ic = (Get-ChildItem $imgs -File).Count
Write-Output ""
Write-Output "  output/images/ : $ic images"
Write-Output "=========================================="
Write-Output "Done!"
