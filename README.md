# SLOTH-GO
<h2>Same as the original, but written in GO</h2>

Enter values for the following in your os args:<br />
<strong>Input Path:</strong> The path of the files you want to move<br />
<strong>Output Path:</strong> The path of where you want the files to move to<br />
<strong>Pattern:</strong> The file extension to search for (csv, docx, txt, xls, zip, etc (all extensions supported))<br />
<strong>Folder Type:</strong> What output file structure to create in your output path<br />
<ul>
<li>By modified date (moddate):  1</li>
<li>By file extension (pattern):  2</li>
<li>Simple move from folder to folder (none):  3</li>
</ul>
  
Example args:<br />
"C:\Users\%userprofile%\Downloads" "C:\Users\%userprofile%\Downloads\Archive" "*.txt" "2"<br />
This will create a "txt" folder in Archive. Example: C:\Users\%userprofile%\Downloads\Archive\txt<br />
  Then it will move all txt files from downloads to downloads\archive\txt<br />
