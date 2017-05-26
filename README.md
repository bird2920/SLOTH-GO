# SLOTH-GO
Same as the original, but written in GO (also minus a couple of features)

Enter values for the following in your os args:
Input Path: The path of the files you want to move
Output Path: The path of where you want the files to move to
Pattern: The file extension to search for (csv, docx, txt, xls, zip, etc (all extensions supported))
Folder Type: What output file structure to create in your output path
  By modified date (moddate): 1
  By file extension (pattern): 2
  Simple move from folder to folder (none): 3
  
Example args:
"C:\Users\%userprofile%\Downloads" "C:\Users\%userprofile%\Downloads\Archive" "*.txt" "2"
This will create a "txt" folder in Archive. Example: C:\Users\%userprofile%\Downloads\Archive\txt
  Then it will move all txt files from downloads to downloads\archive\txt
